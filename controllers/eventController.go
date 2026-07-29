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
	missionTime := event.GetTime()

	switch inner := event.GetEvent().(type) {
	case *mission.StreamEventsResponse_Connect:
		logs.Sugar.Debugf("Connect event: %v", inner.Connect)
		return ConnectEvent(inner.Connect)

	case *mission.StreamEventsResponse_Birth:
		logs.Sugar.Debugf("Birth event: %v", inner.Birth)
		return BirthEvent(inner.Birth)

	case *mission.StreamEventsResponse_Shot:
		logs.Sugar.Debugf("Shot event: %v", inner.Shot)
		return ShotEvent(inner.Shot, missionTime)

	case *mission.StreamEventsResponse_Hit:
		logs.Sugar.Debugf("Hit event: %v", inner.Hit)
		return HitEvent(inner.Hit, missionTime)

	case *mission.StreamEventsResponse_Kill:
		logs.Sugar.Debugf("Kill event: %v", inner.Kill)
		return KillEvent(inner.Kill, missionTime)

	case *mission.StreamEventsResponse_Crash:
		logs.Sugar.Debugf("Crash event: %v", inner.Crash)
		return initiatorEvent("crash", missionTime, inner.Crash.GetInitiator())

	case *mission.StreamEventsResponse_UnitLost:
		logs.Sugar.Debugf("UnitLost event: %v", inner.UnitLost)
		return initiatorEvent("unit_lost", missionTime, inner.UnitLost.GetInitiator())

	case *mission.StreamEventsResponse_Disconnect:
		logs.Sugar.Debugf("Disconnect event: %v", inner.Disconnect)
		return DisconnectEvent(inner.Disconnect)

	case *mission.StreamEventsResponse_PilotDead:
		logs.Sugar.Debugf("PilotDead event: %v", inner.PilotDead)
		return initiatorEvent("pilot_dead", missionTime, inner.PilotDead.GetInitiator())

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

// initiatorEvent stores an event that only identifies the unit it happened to:
// crash, unit_lost and pilot_dead all have this shape.
func initiatorEvent(eventType string, missionTime float64, initiator *common.Initiator) error {
	from := buildInitiator(initiator)

	if from.Unit.Type == "" {
		logs.Sugar.Debugf("Skipping %s event: initiator could not be identified", eventType)
		return nil
	}

	event := models.Event{
		MissionTime:   missionTime,
		Coalition:     from.Coalition,
		InitiatorKind: from.Kind,
	}

	event.FromStreamEventsResponse(eventType, &from.Player, &from.Unit, nil, nil)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store %s event: %v", eventType, err)
		return err
	}

	return nil
}

func ShotEvent(p *mission.StreamEventsResponse_ShotEvent, missionTime float64) error {
	var weapon models.Weapon
	weapon.Type = p.GetWeapon().GetType()

	// Shots come from statics as well as units: a SAM launcher is a static, and
	// resolving only the unit case dropped its initiator entirely.
	from := buildInitiator(p.GetInitiator())

	event := models.Event{
		MissionTime:   missionTime,
		Coalition:     from.Coalition,
		InitiatorKind: from.Kind,
	}

	event.FromStreamEventsResponse("shot", &from.Player, &from.Unit, &weapon, nil)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store shot event: %v", err)
		return err
	}

	return nil
}

// HitEvent records a weapon striking something. It shares its shape with a kill,
// which is what makes accuracy and Pk computable: shots fired versus hits landed
// versus kills achieved.
func HitEvent(p *mission.StreamEventsResponse_HitEvent, missionTime float64) error {
	return engagementEvent("hit", missionTime, p.GetInitiator(), p.GetWeapon(), p.WeaponName, p.GetTarget())
}

func KillEvent(p *mission.StreamEventsResponse_KillEvent, missionTime float64) error {
	return engagementEvent("kill", missionTime, p.GetInitiator(), p.GetWeapon(), p.WeaponName, p.GetTarget())
}

// engagementEvent stores an event describing one thing acting on another with a
// weapon. Hit and kill events are identical in shape, so they share this path.
func engagementEvent(
	eventType string,
	missionTime float64,
	initiator *common.Initiator,
	protoWeapon *common.Weapon,
	weaponName *string,
	protoTarget *common.Target,
) error {
	// Set Weapon. Some events name the weapon without describing it, notably
	// when the "weapon" was an aircraft flown into the ground.
	var weapon models.Weapon
	if protoWeapon != nil {
		weapon.Type = protoWeapon.GetType()
	} else if weaponName != nil {
		weapon.Type = *weaponName
	}

	from := buildInitiator(initiator)

	target, targetCoalition := buildTarget(protoTarget)

	event := models.Event{
		MissionTime:     missionTime,
		Coalition:       from.Coalition,
		InitiatorKind:   from.Kind,
		TargetCoalition: targetCoalition,
	}

	event.FromStreamEventsResponse(eventType, &from.Player, &from.Unit, &weapon, &target)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store %s event: %v", eventType, err)
		return err
	}

	return nil
}

// initiatorInfo describes whatever caused an event. DCS models an initiator
// with the same oneof as a target, so it can be a static SAM site, a weapon or
// scenery, not just a unit.
type initiatorInfo struct {
	Unit      models.Unit
	Kind      string
	Coalition string
	Player    models.Player
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

	case initiator.GetStatic() != nil:
		static := initiator.GetStatic()
		info.Kind = models.ObjectKindStatic
		info.Coalition = models.CoalitionFromProto(static.GetCoalition())
		info.Unit.Type = static.GetType()
		info.Player = models.AIPlayerFor(info.Coalition)

	case initiator.GetWeapon() != nil:
		// A weapon can cause a hit in its own right, for example a bomb
		// detonating after its launcher has already been destroyed.
		info.Kind = models.ObjectKindWeapon
		info.Unit.Type = initiator.GetWeapon().GetType()
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	case initiator.GetScenery() != nil:
		info.Kind = models.ObjectKindScenery
		info.Unit.Type = initiator.GetScenery().GetType()
		info.Player = models.AIPlayerFor(models.CoalitionUnknown)

	case initiator.GetAirbase() != nil:
		airbase := initiator.GetAirbase()
		info.Kind = models.ObjectKindAirbase
		info.Coalition = models.CoalitionFromProto(airbase.GetCoalition())
		info.Unit.Type = airbase.GetName()
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
func buildTarget(protoTarget *common.Target) (models.Target, string) {
	target := models.Target{Kind: models.ObjectKindUnknown}
	coalition := models.CoalitionUnknown

	if protoTarget == nil {
		return target, coalition
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

	case protoTarget.GetWeapon() != nil:
		// Shooting down a missile, for example.
		target.Kind = models.ObjectKindWeapon
		target.Weapon.FromCommonWeapon(protoTarget.GetWeapon())

	case protoTarget.GetStatic() != nil:
		static := protoTarget.GetStatic()
		target.Kind = models.ObjectKindStatic
		target.Unit.Type = static.GetType()
		coalition = models.CoalitionFromProto(static.GetCoalition())

	case protoTarget.GetScenery() != nil:
		// Map objects: buildings, bridges. They belong to no coalition.
		target.Kind = models.ObjectKindScenery
		target.Unit.Type = protoTarget.GetScenery().GetType()

	case protoTarget.GetAirbase() != nil:
		airbase := protoTarget.GetAirbase()
		target.Kind = models.ObjectKindAirbase
		target.Unit.Type = airbase.GetName()
		coalition = models.CoalitionFromProto(airbase.GetCoalition())

	default:
		logs.Sugar.Debugf("Unrecognised target type: %v", protoTarget)
	}

	return target, coalition
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
