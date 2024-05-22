package controllers

import (
	"io"
	"log"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
)

/* func CreateEvent(c *gin.Context) {
	// Get data off req body
	var event models.Event

	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate data

	// Create player
	if result := initializers.DB.Create(&event); result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	// Return player
	c.JSON(http.StatusOK, gin.H{"message": "Event created successfully!", "event": event})
}

func GetEvents(c *gin.Context) {
	var events []models.Event

	initializers.DB.Find(&events)

	c.JSON(http.StatusOK, gin.H{"events": events})
} */

type EventHandler interface {
	HandleEvent(event *mission.StreamEventsResponse) error
}

type DCSEventHandler struct{}

func (d *DCSEventHandler) HandleEvent(event *mission.StreamEventsResponse) error {
	// Handle the event here, using the event handler interface

	switch inner := event.GetEvent().(type) {
	case *mission.StreamEventsResponse_Connect:
		/* log.Printf("Connect event: %v", inner.Connect) */
		err := ConnectEvent(inner.Connect)
		if err != nil {
			return err
		}
		log.Printf("Connect event processed: %v", inner.Connect)

	case *mission.StreamEventsResponse_Birth:
		/* log.Printf("Birth event: %v", inner.Birth) */
		err := BirthEvent(inner.Birth)
		if err != nil {
			return err
		}
		log.Printf("Birth event processed: %v", inner.Birth)

	case *mission.StreamEventsResponse_Shot:
		log.Printf("Shot event: %v", inner.Shot)
		err := ShotEvent(inner.Shot)
		if err != nil {
			return err
		}
		log.Printf("Shot event processed: %v", inner.Shot)

	case *mission.StreamEventsResponse_SimulationFps:
	default:
		log.Printf("Received unknown event type: %T", inner)
	}

	return nil
}

func StreamEvents() {
	var eventHandler EventHandler = &DCSEventHandler{}

	for {
		event, err := initializers.StreamEventsClient.Recv()
		if err == io.EOF {
			log.Printf("Server closed events stream")
		} else if err != nil {
			log.Panicf("Failed to receive event: %v", err)
		}

		/* log.Printf("Received event: %v", event.GetEvent()) */
		err = eventHandler.HandleEvent(event)
		if err != nil {
			log.Panicf("Failed to handle event: %v", err)
		}

	}
}

func ShotEvent(p *mission.StreamEventsResponse_ShotEvent) error {

	log.Printf("Shot event: %v", p)

	// Check if weapon already exists in DB

	weapon, err := FindWeaponByType(p.Weapon.Type)
	if err != nil {
		log.Printf("Failed to find weapon: %v", err)
	}

	if weapon == nil {
		var newWeapon models.Weapon
		newWeapon.FromCommonWeapon(p.Weapon)

		err := CreateWeapon(&newWeapon)
		if err != nil {
			log.Printf("Failed to create weapon: %v", err)
			return err
		}

		log.Printf("Weapon created: %v", newWeapon.Type)

		weapon, err = FindWeaponByType(p.Weapon.Type)
		if err != nil {
			log.Printf("Failed to find weapon: %v", err)
		}
	}

	// Check if player already exists in DB
	var connectedPlayer models.Player

	u := p.Initiator.GetUnit()

	if u.GetPlayerName() != "" {
		connectedPlayer.PlayerName = u.PlayerName
		/* 		if !connectedPlayer.CheckIfPlayerInDB() {
			err := connectedPlayer.CreatePlayer()
			if err != nil {
				log.Printf("Failed to create player: %v", err)
				return err
			}
		} */

		connectedPlayer.GetPlayerUcid()
	} else {
		// If no player is attached to the unit, it's an AI unit
		name := "AI-Unit"
		connectedPlayer.PlayerName = &name
		connectedPlayer.UCID = "0"
		connectedPlayer.PlayerID = 0
		/* 		if !connectedPlayer.CheckIfPlayerInDB() {
			aiPlayer := models.Player{
				PlayerName: &name,
				UCID:       "0",
			}

			err := aiPlayer.CreatePlayer()
			if err != nil {
				log.Printf("Failed to create player: %v", err)
				return err
			}
		} */
	}

	/* 	err = connectedPlayer.GetPlayerFromDB()
	   	if err != nil {
	   		log.Printf("Failed to get player: %v", err)
	   	}

	   	log.Printf("Connected player ID: %v", connectedPlayer.PlayerID)
	   	log.Printf("Connected player: %v", connectedPlayer.GetPlayerName()) */

	// Create event in DB
	initiator := models.Unit{}

	initiator.FromCommonUnit(u)

	event := models.Event{}

	event.FromStreamEventsResponse("shot", &connectedPlayer, &initiator, weapon, &models.Unit{})

	event.CreateEvent()
	if err != nil {
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
		if !connectedPlayer.CheckIfPlayerInDB() {
			err := connectedPlayer.CreatePlayer()
			if err != nil {
				log.Printf("Failed to create player: %v", err)
				return err
			}
		}
	} else {
		// If no player is attached to the unit, it's an AI unit
		name := "AI-Unit"
		connectedPlayer.PlayerName = &name
		if !connectedPlayer.CheckIfPlayerInDB() {
			aiPlayer := models.Player{
				PlayerName: &name,
				UCID:       "0",
			}

			err := aiPlayer.CreatePlayer()
			if err != nil {
				log.Printf("Failed to create player: %v", err)
				return err
			}
		}
	}

	// Check if unit is in DB
	queryResult := initializers.DB.Where("type = ?", u.Type).First(&unit)
	if queryResult.Error != nil && queryResult.Error.Error() != "record not found" {
		log.Printf("Failed to query event count: %v", queryResult.Error)
	}

	// If not, create unit
	if queryResult.RowsAffected == 0 {
		err := CreateUnit(&unit)
		if err != nil {
			log.Printf("Failed to create unit: %v", err)
			return err
		}

		/* log.Printf("Unit created: %v", unit.Name) */
	} else { // If unit is in DB, update unit
		var updatedUnit models.Unit
		updatedUnit.FromCommonUnit(u)

		err := UpdateUnit(&unit, &updatedUnit)
		if err != nil {
			log.Printf("Failed to update unit: %v", err)
			return err
		}

		/* log.Printf("Unit updated: %v", unit.Name) */
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
			log.Printf("Failed to create player: %v", err)
			return err
		}
	} else { // If player is in DB, update player
		updatedPlayer := models.Player{
			UCID:       p.Ucid,
			PlayerName: &p.Name,
		}

		err := connectedPlayer.UpdatePlayer(&updatedPlayer)
		if err != nil {
			log.Printf("Failed to update player: %v", err)
			return err
		}

		/* log.Printf("Player updated: %v", player.GetPlayerName()) */

	}

	return nil
}
