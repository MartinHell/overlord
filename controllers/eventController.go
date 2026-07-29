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

func GetEventsByType(eventType string) []*models.Event {
	var events []*models.Event

	initializers.ApplyPreloads(initializers.DB).Where("event = ?", eventType).Find(&events)

	return events
}

// GetEventsFiltered returns events optionally narrowed by type and by the
// initiator's coalition. An empty value for either means "no filter", which is
// how a caller asks for both sides at once.
func GetEventsFiltered(eventType, coalition string) []*models.Event {
	var events []*models.Event

	query := initializers.ApplyPreloads(initializers.DB)

	if eventType != "" {
		query = query.Where("event = ?", eventType)
	}
	if coalition != "" {
		query = query.Where("coalition = ?", coalition)
	}

	query.Find(&events)

	return events
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
	// Handle the event here, using the event handler interface

	switch inner := event.GetEvent().(type) {
	case *mission.StreamEventsResponse_Connect:
		logs.Sugar.Debugf("Connect event: %v", inner.Connect)
		err := ConnectEvent(inner.Connect)
		if err != nil {
			return err
		}
		logs.Sugar.Debugf("Connect event processed: %v", inner.Connect)

	case *mission.StreamEventsResponse_Birth:
		logs.Sugar.Debugf("Birth event: %v", inner.Birth)
		err := BirthEvent(inner.Birth)
		if err != nil {
			return err
		}
		logs.Sugar.Debugf("Birth event processed: %v", inner.Birth)

	case *mission.StreamEventsResponse_Shot:
		logs.Sugar.Debugf("Shot event: %v", inner.Shot)
		err := ShotEvent(inner.Shot)
		if err != nil {
			return err
		}
		logs.Sugar.Debugf("Shot event processed: %v", inner.Shot)

	case *mission.StreamEventsResponse_Kill:
		logs.Sugar.Debugf("Kill event: %v", inner.Kill)
		err := KillEvent(inner.Kill)
		if err != nil {
			return err
		}
		logs.Sugar.Debugf("Kill event processed: %v", inner.Kill)

	case *mission.StreamEventsResponse_Crash:
		logs.Sugar.Debugf("Crash event: %v", inner.Crash)
		err := CrashEvent(inner.Crash)
		if err != nil {
			return err
		}
		logs.Sugar.Debugf("Crash event processed: %v", inner.Crash)

	case *mission.StreamEventsResponse_SimulationFps:
	default:
		logs.Sugar.Debugf("Received unknown event type: %T", inner)
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

func CrashEvent(p *mission.StreamEventsResponse_CrashEvent) error {
	logs.Sugar.Debugf("Crash event: %v", p)

	u := p.Initiator.GetUnit()

	connectedPlayer := resolvePlayer(u)

	initiator := models.Unit{}
	initiator.FromCommonUnit(u)

	event := models.Event{Coalition: models.CoalitionFromUnit(u)}

	event.FromStreamEventsResponse("crash", &connectedPlayer, &initiator, nil, nil)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store crash event: %v", err)
		return err
	}

	return nil
}

func ShotEvent(p *mission.StreamEventsResponse_ShotEvent) error {

	logs.Sugar.Debugf("Shot event: %v", p)

	// Set Weapon
	var weapon models.Weapon

	weapon.Type = p.GetWeapon().GetType()

	u := p.Initiator.GetUnit()

	connectedPlayer := resolvePlayer(u)

	initiator := models.Unit{}
	initiator.FromCommonUnit(u)

	event := models.Event{Coalition: models.CoalitionFromUnit(u)}

	event.FromStreamEventsResponse("shot", &connectedPlayer, &initiator, &weapon, nil)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store shot event: %v", err)
		return err
	}

	return nil
}

func KillEvent(p *mission.StreamEventsResponse_KillEvent) error {

	logs.Sugar.Debugf("Kill event: %v", p)

	// Set Weapon
	var weapon models.Weapon
	if p.Weapon != nil {
		weapon.Type = p.Weapon.Type
	} else if p.WeaponName != nil {
		weapon.Type = *p.WeaponName
	}

	// Check if player already exists in DB
	var connectedPlayer models.Player
	initiator := models.Unit{}
	coalition := models.CoalitionUnknown

	if p.Initiator != nil {
		u := p.Initiator.GetUnit()

		connectedPlayer = resolvePlayer(u)
		coalition = models.CoalitionFromUnit(u)

		initiator.FromCommonUnit(u)
	}

	// Create event in DB

	// Create target
	target := models.Target{}
	targetCoalition := models.CoalitionUnknown
	if p.Target != nil {
		tgt := p.Target.GetUnit()

		if tgt != nil {
			targetCoalition = models.CoalitionFromUnit(tgt)

			if tgt.GetPlayerName() != "" {
				target.Player.PlayerName = tgt.PlayerName
				target.Player.GetPlayerFromDB()
			}

			target.Unit.FromCommonUnit(tgt)
		} else if p.Target.GetWeapon() != nil {
			target.Weapon.FromCommonWeapon(p.Target.GetWeapon())
		} else {
			// TODO: Handle more target types
			logs.Sugar.Debugf("Unknown target type: %v", p.Target)
		}
	}

	event := models.Event{Coalition: coalition, TargetCoalition: targetCoalition}

	event.FromStreamEventsResponse("kill", &connectedPlayer, &initiator, &weapon, &target)

	if err := event.CreateEvent(); err != nil {
		logs.Sugar.Errorf("Failed to store kill event: %v", err)
		return err
	}

	return nil
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

// ConnectEvent handles the connect event
func ConnectEvent(p *mission.StreamEventsResponse_ConnectEvent) error {
	var connectedPlayer models.Player

	connectedPlayer.FromStreamEventsResponse_ConnectEvent(p)

	// The connect event carries the UCID directly, so the player can be stored
	// without consulting the server's player list.
	if err := connectedPlayer.EnsureInDB(); err != nil {
		logs.Sugar.Errorf(logCreatePlayer, err)
		return err
	}

	// Reconnecting under a new name is common, so refresh the stored details.
	updatedPlayer := models.Player{
		UCID:       p.GetUcid(),
		PlayerName: &p.Name,
	}

	if err := connectedPlayer.UpdatePlayer(&updatedPlayer); err != nil {
		logs.Sugar.Errorf("Failed to update player: %v", err)
		return err
	}

	logs.Sugar.Debugf("Player connected: %v", connectedPlayer.GetPlayerName())

	return nil
}
