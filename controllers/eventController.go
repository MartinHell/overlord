package controllers

import (
	"io"
	"time"

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

	initializers.DB.Preload("Player").Preload("Initiator").Preload("Target").Preload("Weapon").Find(&events)

	return events
}

func GetEventsByType(eventType string) []*models.Event {
	var events []*models.Event

	initializers.DB.Preload("Player").Preload("Initiator").Preload("Target").Preload("Weapon").Where("event = ?", eventType).Find(&events)

	return events
}

func GetEventsByTypeAndPlayer(eventType string, playerID uint) []*models.Event {
	var events []*models.Event

	initializers.DB.Preload("Player").Preload("Initiator").Preload("Target").Preload("Weapon").Where("event = ? AND player_id = ?", eventType, playerID).Find(&events)

	return events
}

func GetEvent(id string) *models.Event {
	var event *models.Event

	initializers.DB.Preload("Player").Preload("Initiator").Preload("Target").Preload("Weapon").First(&event, id)

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
	case *mission.StreamEventsResponse_SimulationFps:
	default:
		logs.Sugar.Debugf("Received unknown event type: %T", inner)
	}

	return nil
}

func StreamEvents() {
	var eventHandler EventHandler = &DCSEventHandler{}

	for {
		err := handleStreamEvents(eventHandler)
		if err == io.EOF {
			logs.Sugar.Errorf("Server closed events stream, retrying...")
		} else if err != nil {
			logs.Sugar.Errorf("Failed to receive event: %v, retrying...", err)
		}

		// Wait before retrying to avoid tight loop
		time.Sleep(5 * time.Second)
	}
}

func handleStreamEvents(eventHandler EventHandler) error {
	for {
		event, err := initializers.StreamEventsClient.Recv()
		if err != nil {
			return err
		}

		logs.Sugar.Debugf("Received event: %v", event.Event)
		err = eventHandler.HandleEvent(event)
		if err != nil {
			logs.Sugar.Errorf("Failed to handle event: %v", err)
		}
	}
}

func ShotEvent(p *mission.StreamEventsResponse_ShotEvent) error {

	logs.Sugar.Debugf("Shot event: %v", p)

	// Set Weapon
	var weapon models.Weapon

	weapon.Type = p.Weapon.Type

	// Check if player already exists in DB
	var connectedPlayer models.Player

	u := p.Initiator.GetUnit()

	if u.GetPlayerName() != "" {
		connectedPlayer.PlayerName = u.PlayerName

		connectedPlayer.GetPlayerFromDB()
	} else {
		// If no player is attached to the unit, it's an AI unit
		connectedPlayer = models.AIPlayer
	}

	// Create event in DB
	initiator := models.Unit{}

	initiator.FromCommonUnit(u)

	event := models.Event{}

	event.FromStreamEventsResponse("shot", &connectedPlayer, &initiator, &weapon, nil, nil)

	event.CreateEvent()

	return nil
}

func KillEvent(p *mission.StreamEventsResponse_KillEvent) error {

	logs.Sugar.Debugf("Kill event: %v", p)

	// Set Weapon
	var weapon models.Weapon
	if p.Weapon != nil {
		weapon.Type = p.Weapon.Type
	} else {
		weapon.Type = *p.WeaponName
	}

	// Check if player already exists in DB
	var connectedPlayer models.Player
	initiator := models.Unit{}

	if p.Initiator != nil {
		u := p.Initiator.GetUnit()

		if u.GetPlayerName() != "" {
			connectedPlayer.PlayerName = u.PlayerName

			connectedPlayer.GetPlayerFromDB()
		} else {
			// If no player is attached to the unit, it's an AI unit
			connectedPlayer = models.AIPlayer
		}

		initiator.FromCommonUnit(u)
	}

	// Create event in DB

	// Create target
	target := models.Unit{}
	targetWeapon := models.Weapon{}
	if p.Target != nil {
		if p.Target.GetUnit() != nil {
			target.FromCommonUnit(p.Target.GetUnit())
		} else if p.Target.GetWeapon() != nil {
			targetWeapon.Type = p.Target.GetWeapon().GetType()
		} else {
			// TODO: Handle more target types
			logs.Sugar.Debugf("Unknown target type: %v", p.Target)
		}
	}

	event := models.Event{}

	event.FromStreamEventsResponse("kill", &connectedPlayer, &initiator, &weapon, &target, &targetWeapon)

	event.CreateEvent()

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
		if !connectedPlayer.CheckIfPlayerInDB() {
			err := connectedPlayer.CreatePlayer()
			if err != nil {
				logs.Sugar.Errorf(logCreatePlayer, err)
				return err
			}
		}
	} else {
		// If no player is attached to the unit, it's an AI unit
		if !models.AIPlayer.CheckIfPlayerInDB() {

			err := models.AIPlayer.CreatePlayer()
			if err != nil {
				logs.Sugar.Errorf(logCreatePlayer, err)
				return err
			}
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

	// Check if player is in DB
	if p.GetName(); !connectedPlayer.CheckIfPlayerInDB() {
		err := connectedPlayer.CreatePlayer()
		if err != nil {
			logs.Sugar.Errorf(logCreatePlayer, err)
			return err
		}
	} else { // If player is in DB, update player
		updatedPlayer := models.Player{
			UCID:       p.Ucid,
			PlayerName: &p.Name,
		}

		err := connectedPlayer.UpdatePlayer(&updatedPlayer)
		if err != nil {
			logs.Sugar.Errorf("Failed to update player: %v", err)
			return err
		}

		logs.Sugar.Debugf("Player updated: %v", connectedPlayer.GetPlayerName())

	}

	return nil
}
