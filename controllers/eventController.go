package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime/debug"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

type EventHandler interface {
	HandleEvent(event *mission.StreamEventsResponse) error
}

type DCSEventHandler struct{}

func GetEvents() []*models.Event {
	var events []*models.Event

	initializers.ApplyPreloads(initializers.DB).Find(&events)

	return events
}

const (
	// defaultPageSize applies when a query omits the optional first argument.
	defaultPageSize = 50
	// maxPageSize caps what a single query can pull back, so a client cannot
	// ask for the whole table in one request.
	maxPageSize = 500
)

func GetEventsByType(eventType string) []*models.Event {
	var events []*models.Event

	initializers.ApplyPreloads(initializers.DB).Where("event = ?", eventType).Find(&events)

	return events
}

// EventPage is one page of events plus what the caller needs to ask for the
// next one.
type EventPage struct {
	Events      []*models.Event
	HasNextPage bool
}

// GetEventsPage returns newest-first events, narrowed by type and by the
// initiator's coalition. An empty value for either means "no filter", which is
// how a caller asks for both sides at once.
//
// Paging is keyset rather than offset: `after` is an event ID, and because the
// ordering is a descending primary key, the next page is simply everything with
// a smaller ID. That stays correct while new events are being written, which an
// OFFSET would not, and it never loads more than one page into memory.
func GetEventsPage(eventType, coalition string, first int, after string) (EventPage, error) {
	if first <= 0 {
		first = defaultPageSize
	}
	if first > maxPageSize {
		first = maxPageSize
	}

	query := initializers.ApplyPreloads(initializers.DB).Model(&models.Event{})

	if eventType != "" {
		query = query.Where("event = ?", eventType)
	}
	if coalition != "" {
		query = query.Where("coalition = ?", coalition)
	}
	if after != "" {
		query = query.Where("id < ?", after)
	}

	var events []*models.Event

	// Fetch one extra row to find out whether another page exists, without a
	// second COUNT query over the whole table.
	if err := query.Order("id DESC").Limit(first + 1).Find(&events).Error; err != nil {
		logs.Sugar.Errorf("Failed to query events: %v", err)
		return EventPage{}, err
	}

	page := EventPage{Events: events}
	if len(events) > first {
		page.Events = events[:first]
		page.HasNextPage = true
	}

	return page, nil
}

// GetKillsByCoalition tallies kill events per initiating coalition, covering
// both sides in a single query.
func GetKillsByCoalition() []*models.CoalitionKills {
	var events []*models.Event

	initializers.DB.Where("event = ?", "kill").Find(&events)

	order := []string{}
	tally := map[string]*models.CoalitionKills{}

	for _, event := range events {
		coalition := event.Coalition
		if coalition == "" {
			coalition = models.CoalitionUnknown
		}

		if tally[coalition] == nil {
			tally[coalition] = &models.CoalitionKills{Coalition: coalition}
			order = append(order, coalition)
		}

		tally[coalition].Kills++

		// Only count a teamkill when both sides are actually known. Two unknown
		// coalitions compare equal but say nothing about whose side anyone was
		// on, and historical events predating coalition tracking are all
		// unknown.
		if coalition != models.CoalitionUnknown && event.TargetCoalition == coalition {
			tally[coalition].Teamkills++
		}
	}

	result := make([]*models.CoalitionKills, 0, len(order))
	for _, coalition := range order {
		result = append(result, tally[coalition])
	}

	return result
}

func GetEventsByTypeAndPlayer(eventType string, playerID uint) []*models.Event {
	var events []*models.Event

	initializers.ApplyPreloads(initializers.DB).Where("event = ? AND player_id = ?", eventType, playerID).Find(&events)

	return events
}

func GetEvent(id string) *models.Event {
	var event *models.Event

	initializers.ApplyPreloads(initializers.DB).First(&event, id)

	return event
}

func (d *DCSEventHandler) HandleEvent(event *mission.StreamEventsResponse) error {
	// Every event carries the mission clock, which is the only timestamp that
	// survives an overlord restart or lines up with a track file.
	t := event.GetTime()

	switch inner := event.GetEvent().(type) {
	case *mission.StreamEventsResponse_Connect:
		return ConnectEvent(inner.Connect)

	case *mission.StreamEventsResponse_Disconnect:
		return DisconnectEvent(inner.Disconnect)

	case *mission.StreamEventsResponse_Birth:
		return BirthEvent(inner.Birth)

	case *mission.StreamEventsResponse_Shot:
		return recordEvent(eventDetail{Type: "shot", MissionTime: t,
			Initiator: inner.Shot.GetInitiator(), Weapon: inner.Shot.GetWeapon()})

	case *mission.StreamEventsResponse_Hit:
		if isCollateralHit(inner.Hit) {
			logs.Sugar.Debugf("Skipping collateral hit on %v", inner.Hit.GetTarget())
			return nil
		}
		return recordEvent(eventDetail{Type: "hit", MissionTime: t,
			Initiator: inner.Hit.GetInitiator(), Weapon: inner.Hit.GetWeapon(),
			WeaponName: inner.Hit.WeaponName, Target: inner.Hit.GetTarget()})

	case *mission.StreamEventsResponse_Kill:
		return recordEvent(eventDetail{Type: "kill", MissionTime: t,
			Initiator: inner.Kill.GetInitiator(), Weapon: inner.Kill.GetWeapon(),
			WeaponName: inner.Kill.WeaponName, Target: inner.Kill.GetTarget()})

	case *mission.StreamEventsResponse_Crash:
		return recordEvent(eventDetail{Type: "crash", MissionTime: t, Initiator: inner.Crash.GetInitiator()})

	case *mission.StreamEventsResponse_UnitLost:
		return recordEvent(eventDetail{Type: "unit_lost", MissionTime: t, Initiator: inner.UnitLost.GetInitiator()})

	case *mission.StreamEventsResponse_PilotDead:
		return recordEvent(eventDetail{Type: "pilot_dead", MissionTime: t, Initiator: inner.PilotDead.GetInitiator()})

	case *mission.StreamEventsResponse_Dead:
		return recordEvent(eventDetail{Type: "dead", MissionTime: t, Initiator: inner.Dead.GetInitiator()})

	case *mission.StreamEventsResponse_Ejection:
		return recordEvent(eventDetail{Type: "ejection", MissionTime: t,
			Initiator: inner.Ejection.GetInitiator(), Target: inner.Ejection.GetTarget()})

	// Sortie lifecycle. These are what make a sortie, and therefore per-sortie
	// stats, possible at all.
	case *mission.StreamEventsResponse_Takeoff:
		return recordEvent(eventDetail{Type: "takeoff", MissionTime: t,
			Initiator: inner.Takeoff.GetInitiator(), Place: inner.Takeoff.GetPlace()})

	case *mission.StreamEventsResponse_RunwayTakeoff:
		return recordEvent(eventDetail{Type: "runway_takeoff", MissionTime: t,
			Initiator: inner.RunwayTakeoff.GetInitiator(), Place: inner.RunwayTakeoff.GetPlace()})

	case *mission.StreamEventsResponse_Land:
		return recordEvent(eventDetail{Type: "land", MissionTime: t,
			Initiator: inner.Land.GetInitiator(), Place: inner.Land.GetPlace()})

	case *mission.StreamEventsResponse_RunwayTouch:
		return recordEvent(eventDetail{Type: "runway_touch", MissionTime: t,
			Initiator: inner.RunwayTouch.GetInitiator(), Place: inner.RunwayTouch.GetPlace()})

	case *mission.StreamEventsResponse_EngineStartup:
		return recordEvent(eventDetail{Type: "engine_startup", MissionTime: t,
			Initiator: inner.EngineStartup.GetInitiator(), Place: inner.EngineStartup.GetPlace()})

	case *mission.StreamEventsResponse_EngineShutdown:
		return recordEvent(eventDetail{Type: "engine_shutdown", MissionTime: t,
			Initiator: inner.EngineShutdown.GetInitiator(), Place: inner.EngineShutdown.GetPlace()})

	// Slot occupancy. PlayerChangeSlot arrives first with the player identity;
	// PlayerEnterUnit follows seconds later with the airframe.
	case *mission.StreamEventsResponse_PlayerEnterUnit:
		return recordEvent(eventDetail{Type: "player_enter_unit", MissionTime: t,
			Initiator: inner.PlayerEnterUnit.GetInitiator()})

	case *mission.StreamEventsResponse_PlayerLeaveUnit:
		return recordEvent(eventDetail{Type: "player_leave_unit", MissionTime: t,
			Initiator: inner.PlayerLeaveUnit.GetInitiator()})

	case *mission.StreamEventsResponse_PlayerChangeSlot:
		return PlayerChangeSlotEvent(inner.PlayerChangeSlot, t)

	// The landing grade arrives as free text in Comment.
	case *mission.StreamEventsResponse_LandingQualityMark:
		return recordEvent(eventDetail{Type: "landing_quality_mark", MissionTime: t,
			Initiator: inner.LandingQualityMark.GetInitiator(),
			Place:     inner.LandingQualityMark.GetPlace(),
			Comment:   inner.LandingQualityMark.GetComment()})

	// Loadout: what was carried, against what was later expended.
	case *mission.StreamEventsResponse_WeaponAdd:
		name := inner.WeaponAdd.GetWeaponName()
		return recordEvent(eventDetail{Type: "weapon_add", MissionTime: t,
			Initiator: inner.WeaponAdd.GetInitiator(), WeaponName: &name})

	case *mission.StreamEventsResponse_BaseCapture:
		return recordEvent(eventDetail{Type: "base_capture", MissionTime: t,
			Initiator: inner.BaseCapture.GetInitiator(), Place: inner.BaseCapture.GetPlace()})

	// Mission boundaries carry no payload beyond the clock, but without them
	// every event ever recorded is one undifferentiated pile.
	case *mission.StreamEventsResponse_MissionStart:
		return recordEvent(eventDetail{Type: "mission_start", MissionTime: t})

	case *mission.StreamEventsResponse_MissionEnd:
		return recordEvent(eventDetail{Type: "mission_end", MissionTime: t})

	case *mission.StreamEventsResponse_SimulationFps:
	default:
		logs.Sugar.Debugf("Received unhandled event type: %T", inner)
	}

	return nil
}

const (
	// streamBaseDelay is the delay before the first reconnect attempt.
	streamBaseDelay = 2 * time.Second
	// streamMaxDelay caps the exponential backoff between reconnect attempts.
	streamMaxDelay = 60 * time.Second
	// streamStableAfter is how long a stream must stay up before it counts as
	// healthy and the backoff is reset. Without this a stream that dies
	// immediately after every reconnect would keep resetting to the base delay.
	streamStableAfter = 30 * time.Second
)

// StreamEvents consumes DCS mission events until ctx is cancelled, reconnecting
// whenever the stream drops (mission restart, DCS shutdown, network blip).
func StreamEvents(ctx context.Context) {
	var eventHandler EventHandler = &DCSEventHandler{}

	attempt := 0

	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		err := runEventStream(ctx, eventHandler)

		if ctx.Err() != nil {
			return
		}

		if time.Since(start) >= streamStableAfter {
			attempt = 0
		}

		switch {
		case errors.Is(err, io.EOF):
			logs.Sugar.Warnln("DCS closed the events stream")
		default:
			logs.Sugar.Errorf("Events stream failed: %v", err)
		}

		delay := exponentialBackoff(attempt, streamBaseDelay, streamMaxDelay)
		attempt++
		logs.Sugar.Warnf("Reopening the events stream in %v", delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// runEventStream opens a fresh stream and pumps events from it until it fails.
// The stream is deliberately opened here rather than once at startup: a gRPC
// stream is dead for good once Recv returns an error, so reusing one would mean
// never recovering from a DCS restart.
func runEventStream(ctx context.Context, eventHandler EventHandler) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := initializers.OpenEventStream(streamCtx)
	if err != nil {
		return err
	}

	// Player ids are only meaningful within one connection to DCS, so anything
	// tracked from a previous stream is stale.
	resetSessions()
	models.Players.Invalidate()

	logs.Sugar.Infoln("Events stream opened")

	for {
		event, err := stream.Recv()
		if err != nil {
			return err
		}

		logs.Sugar.Debugf("Received event: %v", event.Event)
		if err := handleEventSafely(eventHandler, event); err != nil {
			logs.Sugar.Errorf("Failed to handle event: %v", err)
		}
	}
}

// handleEventSafely turns a panic in an event handler into an error. The stream
// runs on its own goroutine, so without this a single malformed event would
// take the whole process down instead of just skipping that event.
func handleEventSafely(eventHandler EventHandler, event *mission.StreamEventsResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic while handling %T event: %v", event.GetEvent(), r)
			logs.Sugar.Errorf("%v\n%s", err, debug.Stack())
		}
	}()

	return eventHandler.HandleEvent(event)
}

// exponentialBackoff returns base * 2^attempt, capped at max.
func exponentialBackoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	delay := float64(base) * math.Pow(2, float64(attempt))
	if delay >= float64(max) || math.IsInf(delay, 0) {
		return max
	}

	return time.Duration(delay)
}

// resolvePlayer maps the unit behind an event to a player record. A unit flown
// by a human resolves to that human; everything else is attributed to the
// synthetic AI player for the unit's coalition, so red and blue AI are tracked
// as separate players rather than lumped together.
func resolvePlayer(u *common.Unit) models.Player {
	if u.GetPlayerName() != "" {
		var player models.Player
		player.PlayerName = u.PlayerName
		if err := player.GetPlayerFromDB(); err != nil {
			logs.Sugar.Errorf("Failed to resolve player %q: %v", u.GetPlayerName(), err)
		}
		return player
	}

	return models.AIPlayerFor(models.CoalitionFromUnit(u))
}

// eventDetail is everything an event might carry. Using one struct keeps a
// single record path for two dozen event types instead of a near-duplicate
// function per type.
type eventDetail struct {
	Type        string
	MissionTime float64
	Initiator   *common.Initiator
	Target      *common.Target
	Weapon      *common.Weapon
	WeaponName  *string
	Place       *common.Airbase
	Comment     string
	SlotID      string
}

// recordEvent resolves everything an event references and stores one row.
func recordEvent(d eventDetail) error {
	from := buildInitiator(d.Initiator)

	// Some events name the weapon without describing it, notably when the
	// "weapon" was an aircraft flown into the ground.
	var weapon models.Weapon
	if d.Weapon != nil {
		weapon.Type = d.Weapon.GetType()
	} else if d.WeaponName != nil {
		weapon.Type = *d.WeaponName
	}

	target, targetCoalition, targetPos := buildTarget(d.Target)

	event := models.Event{
		MissionTime:       d.MissionTime,
		Coalition:         from.Coalition,
		InitiatorKind:     from.Kind,
		InitiatorName:     from.Name,
		InitiatorGroup:    from.Group,
		InitiatorCallsign: from.Callsign,
		InitiatorLat:      from.Position.Lat,
		InitiatorLon:      from.Position.Lon,
		InitiatorAlt:      from.Position.Alt,
		TargetCoalition:   targetCoalition,
		TargetName:        targetPos.Name,
		TargetLat:         targetPos.Lat,
		TargetLon:         targetPos.Lon,
		TargetAlt:         targetPos.Alt,
		Place:             d.Place.GetName(),
		Comment:           d.Comment,
		SlotID:            d.SlotID,
	}

	event.FromStreamEventsResponse(d.Type, &from.Player, &from.Unit, &weapon, &target)

	// Mission boundaries legitimately carry nothing but the clock. Anything
	// else with no usable content would just be a junk row.
	if isEmptyEvent(d, from, weapon, target) {
		logs.Sugar.Debugf("Skipping %s event: nothing identifiable in it", d.Type)
		return nil
	}

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store %s event: %v", d.Type, err)
		return err
	}

	return nil
}

// isEmptyEvent reports whether an event would store nothing worth keeping.
func isEmptyEvent(d eventDetail, from initiatorInfo, weapon models.Weapon, target models.Target) bool {
	switch d.Type {
	case "mission_start", "mission_end":
		// These are markers; the timestamp is the whole point.
		return false
	}

	return from.Unit.Type == "" &&
		weapon.Type == "" &&
		target.Unit.Type == "" &&
		target.Weapon.Type == "" &&
		d.Place.GetName() == "" &&
		d.Comment == "" &&
		d.SlotID == ""
}

// PlayerChangeSlotEvent records a player taking a slot. It carries the player
// id, coalition and slot but no unit; PlayerEnterUnit follows seconds later
// with the airframe.
func PlayerChangeSlotEvent(p *mission.StreamEventsResponse_PlayerChangeSlotEvent, missionTime float64) error {
	player, known := peekSession(p.GetPlayerId())
	if !known {
		// Overlord may have started mid-mission and never seen the connect.
		logs.Sugar.Debugf("Slot change for unknown session id %d", p.GetPlayerId())
	}

	event := models.Event{
		MissionTime: missionTime,
		Coalition:   models.CoalitionFromProto(p.GetCoalition()),
		SlotID:      p.GetSlotId(),
	}

	event.FromStreamEventsResponse("player_change_slot", &player, &models.Unit{}, nil, nil)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store player_change_slot event: %v", err)
		return err
	}

	return nil
}

// isCollateralHit reports whether a hit is splash damage on a map object rather
// than something worth recording.
//
// DCS emits a hit per scenery object caught in a blast, with neither an
// initiator nor a weapon, so there is nothing to attribute it to: no player, no
// coalition, no weapon to score. In a measured session these were 408 of 500
// events, essentially all of them trees, and each one also created a units row
// for the scenery type.
//
// The test is deliberately "no initiator and no weapon" rather than "target is
// scenery": a scenery hit that DCS does attribute still tells us someone put
// ordnance somewhere, and is kept.
func isCollateralHit(p *mission.StreamEventsResponse_HitEvent) bool {
	if p.GetInitiator() != nil {
		return false
	}

	return p.GetWeapon() == nil && p.GetWeaponName() == ""
}

// initiatorInfo describes whatever caused an event. DCS models an initiator
// with the same oneof as a target, so it can be a static SAM site, a weapon or
// scenery, not just a unit.
type initiatorInfo struct {
	Unit      models.Unit
	Kind      string
	Coalition string
	Player    models.Player
	// Identity of the specific unit, as opposed to its type. Unit rows are
	// deduplicated by type, so this is the only place instance identity lives.
	Name     string
	Group    string
	Callsign string
	Position objectPosition
}

// objectPosition is where something was when an event fired, plus its name.
type objectPosition struct {
	Name string
	Lat  float64
	Lon  float64
	Alt  float64
}

func positionOf(p *common.Position) objectPosition {
	if p == nil {
		return objectPosition{}
	}
	return objectPosition{Lat: p.GetLat(), Lon: p.GetLon(), Alt: p.GetAlt()}
}

// buildInitiator resolves an initiator of any kind. Only the unit case used to
// be handled, so an event started by anything else was stored with an empty
// initiator and attributed to the AI player by accident.
func buildInitiator(initiator *common.Initiator) initiatorInfo {
	info := initiatorInfo{
		Kind:      models.ObjectKindUnknown,
		Coalition: models.CoalitionUnknown,
	}

	if initiator == nil {
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)
		return info
	}

	switch {
	case initiator.GetUnit() != nil:
		u := initiator.GetUnit()
		info.Kind = models.ObjectKindUnit
		info.Coalition = models.CoalitionFromUnit(u)
		info.Player = resolvePlayer(u)
		info.Unit.FromCommonUnit(u)
		info.Name = u.GetName()
		info.Callsign = u.GetCallsign()
		info.Group = u.GetGroup().GetName()
		info.Position = positionOf(u.GetPosition())

	case initiator.GetStatic() != nil:
		static := initiator.GetStatic()
		info.Kind = models.ObjectKindStatic
		info.Coalition = models.CoalitionFromProto(static.GetCoalition())
		info.Unit.Type = static.GetType()
		info.Name = static.GetName()
		info.Position = positionOf(static.GetPosition())
		info.Player = models.AIPlayerFor(info.Coalition)

	case initiator.GetWeapon() != nil:
		// A weapon can cause a hit in its own right, for example a bomb
		// detonating after its launcher has already been destroyed.
		info.Kind = models.ObjectKindWeapon
		info.Unit.Type = initiator.GetWeapon().GetType()
		info.Position = positionOf(initiator.GetWeapon().GetPosition())
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	case initiator.GetScenery() != nil:
		info.Kind = models.ObjectKindScenery
		info.Unit.Type = initiator.GetScenery().GetType()
		info.Position = positionOf(initiator.GetScenery().GetPosition())
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	case initiator.GetAirbase() != nil:
		airbase := initiator.GetAirbase()
		info.Kind = models.ObjectKindAirbase
		info.Coalition = models.CoalitionFromProto(airbase.GetCoalition())
		info.Unit.Type = airbase.GetName()
		info.Name = airbase.GetName()
		info.Position = positionOf(airbase.GetPosition())
		info.Player = models.AIPlayerFor(info.Coalition)

	case initiator.GetCargo() != nil:
		// Cargo carries no identifying fields, so only the kind is recordable.
		info.Kind = models.ObjectKindCargo
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	case initiator.GetUnknown() != nil:
		// DCS could not classify the object but still names it, which beats
		// discarding it.
		info.Unit.Type = initiator.GetUnknown().GetName()
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	default:
		logs.Sugar.Debugf("Unrecognised initiator type: %v", initiator)
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)
	}

	return info
}

// buildTarget converts whatever a target turned out to be into a Target row.
// Every branch records something: previously anything that was not a unit was
// discarded, which lost the target on the great majority of air-to-ground kills.
func buildTarget(protoTarget *common.Target) (models.Target, string, objectPosition) {
	target := models.Target{Kind: models.ObjectKindUnknown}
	coalition := models.CoalitionUnknown
	var pos objectPosition

	if protoTarget == nil {
		return target, coalition, pos
	}

	switch {
	case protoTarget.GetUnit() != nil:
		tgt := protoTarget.GetUnit()
		target.Kind = models.ObjectKindUnit
		coalition = models.CoalitionFromUnit(tgt)

		if tgt.GetPlayerName() != "" {
			target.Player.PlayerName = tgt.PlayerName
			if err := target.Player.GetPlayerFromDB(); err != nil {
				logs.Sugar.Errorf("Failed to resolve target player: %v", err)
			}
		}

		target.Unit.FromCommonUnit(tgt)
		pos = positionOf(tgt.GetPosition())
		pos.Name = tgt.GetName()

	case protoTarget.GetWeapon() != nil:
		// Shooting down a missile, for example.
		target.Kind = models.ObjectKindWeapon
		target.Weapon.FromCommonWeapon(protoTarget.GetWeapon())
		pos = positionOf(protoTarget.GetWeapon().GetPosition())

	case protoTarget.GetStatic() != nil:
		static := protoTarget.GetStatic()
		target.Kind = models.ObjectKindStatic
		target.Unit.Type = static.GetType()
		coalition = models.CoalitionFromProto(static.GetCoalition())
		pos = positionOf(static.GetPosition())
		pos.Name = static.GetName()

	case protoTarget.GetScenery() != nil:
		// Map objects: buildings, bridges. They belong to no coalition.
		target.Kind = models.ObjectKindScenery
		target.Unit.Type = protoTarget.GetScenery().GetType()
		pos = positionOf(protoTarget.GetScenery().GetPosition())

	case protoTarget.GetAirbase() != nil:
		airbase := protoTarget.GetAirbase()
		target.Kind = models.ObjectKindAirbase
		target.Unit.Type = airbase.GetName()
		coalition = models.CoalitionFromProto(airbase.GetCoalition())
		pos = positionOf(airbase.GetPosition())
		pos.Name = airbase.GetName()

	default:
		logs.Sugar.Debugf("Unrecognised target type: %v", protoTarget)
	}

	return target, coalition, pos
}

func BirthEvent(p *mission.StreamEventsResponse_BirthEvent) error {
	var unit models.Unit
	var connectedPlayer models.Player

	if p.Initiator.GetStatic() != nil {
		return nil
	}

	u := p.Initiator.GetUnit()

	unit.FromCommonUnit(u)

	// Check if a player is attached to the unit and if so create them

	if u.GetPlayerName() != "" {
		connectedPlayer.PlayerName = u.PlayerName
		if err := connectedPlayer.EnsureInDB(); err != nil {
			logs.Sugar.Errorf(logCreatePlayer, err)
			return err
		}
	} else {
		// No player attached means an AI unit, tracked per coalition.
		aiPlayer := models.AIPlayerFor(models.CoalitionFromUnit(u))
		if err := aiPlayer.EnsureInDB(); err != nil {
			logs.Sugar.Errorf(logCreatePlayer, err)
			return err
		}
	}

	// Check if unit is in DB
	queryResult := initializers.DB.Where("type = ?", u.Type).First(&unit)
	if queryResult.Error != nil && queryResult.Error.Error() != "record not found" {
		logs.Sugar.Errorf("Failed to query event count: %v", queryResult.Error)
	}

	// If not, create unit
	if queryResult.RowsAffected == 0 {
		err := CreateUnit(&unit)
		if err != nil {
			logs.Sugar.Errorf("Failed to create unit: %v", err)
			return err
		}

		logs.Sugar.Infof("Unit created: %v", unit.Type)
	} else { // If unit is in DB, update unit
		var updatedUnit models.Unit
		updatedUnit.FromCommonUnit(u)

		err := UpdateUnit(&unit, &updatedUnit)
		if err != nil {
			logs.Sugar.Errorf("Failed to update unit: %v", err)
			return err
		}

		logs.Sugar.Debugf("Unit updated: %v", unit.Type)
	}

	return nil
}
