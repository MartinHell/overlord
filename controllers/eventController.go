package controllers

import (
	"io"
	"log"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
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

func StreamEvents(stream mission.MissionService_StreamEventsClient) {
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			log.Printf("Server closed events stream")
		} else if err != nil {
			log.Panicf("Failed to receive event: %v", err)
		}

		switch inner := event.GetEvent().(type) {
		case *mission.StreamEventsResponse_Connect:
			log.Printf("Connect event: %v", inner.Connect)
			err := ConnectEvent(inner.Connect)
			if err != nil {
				log.Panicf("Failed to handle connect event: %v", err)
			}
			log.Printf("Connect event processed: %v", inner.Connect)

		case *mission.StreamEventsResponse_Birth:
			log.Printf("Birth event: %v", inner.Birth)
			err := BirthEvent(inner.Birth)
			if err != nil {
				log.Panicf("Failed to handle birth event: %v", err)
			}
			log.Printf("Birth event processed: %v", inner.Birth)
		}
	}
}

func BirthEvent(p *mission.StreamEventsResponse_BirthEvent) error {
	var unit models.Unit

	u := p.Initiator.GetUnit()
	// Check if unit is in DB
	result := initializers.DB.Where("id = ?", u.Id).First(&unit)
	if result.Error != nil && result.Error.Error() != "record not found" {
		log.Printf("Failed to query event count: %v", result.Error)
	}

	// Check if player is in DB
	var player models.Player
	player.PlayerName = u.PlayerName

	if player.GetPlayerUcid() != "" {
		result := initializers.DB.Where("ucid = ?", player.GetPlayerUcid()).First(&player)
		if result.Error != nil && result.Error.Error() != "record not found" {
			log.Printf("Failed to query event count: %v", result.Error)
		}

		// If not, create player

	}

	// If not, create unit
	if result.RowsAffected == 0 {
		unit.UnitID = 0
		unit.Id = u.GetId()
		unit.Name = u.GetName()
		unit.Type = u.GetType()
		unit.Coalition = u.GetCoalition()
		unit.Position = &common.Position{
			Lat: u.GetPosition().GetLat(),
			Lon: u.GetPosition().GetLon(),
			Alt: u.GetPosition().GetAlt(),
		}
		unit.Callsign = u.GetCallsign()
		unit.Group = u.GetGroup()

		err := CreateUnit(&unit)
		if err != nil {
			log.Printf("Failed to create unit: %v", err)
			return err
		}

		log.Printf("Unit created: %v", unit.Name)
	} else { // If unit is in DB, update unit
		updatedUnit := models.Unit{
			Position: &common.Position{
				Lat: u.GetPosition().GetLat(),
				Lon: u.GetPosition().GetLon(),
				Alt: u.GetPosition().GetAlt(),
			},
			Id:        u.GetId(),
			Name:      u.GetName(),
			Type:      u.GetType(),
			Coalition: u.GetCoalition(),
			Callsign:  u.GetCallsign(),
			Group:     u.GetGroup(),
		}

		err := UpdateUnit(&unit, &updatedUnit)
		if err != nil {
			log.Printf("Failed to update unit: %v", err)
			return err
		}

		log.Printf("Unit updated: %v", unit.Name)
	}

	return nil
}

// ConnectEvent handles the connect event
func ConnectEvent(p *mission.StreamEventsResponse_ConnectEvent) error {
	var player models.Player

	// Check if player is in DB
	result := initializers.DB.Where("uc_id = ?", p.Ucid).First(&player)
	if result.Error != nil && result.Error.Error() != "record not found" {
		log.Printf("Failed to find player: %v", result.Error)
	}

	// If not, create player
	if result.RowsAffected == 0 {

		player.UCID = p.Ucid
		player.Name = p.Name
		player.IP = p.Addr
		player.Id = p.Id

		err := CreatePlayer(&player)
		if err != nil {
			log.Printf("Failed to create player: %v", err)
			return err
		}

		log.Printf("Player created: %v", player.GetPlayerName())
	} else { // If player is in DB, update player
		var updatedPlayer models.Player

		updatedPlayer.UCID = p.Ucid
		updatedPlayer.Name = p.Name
		updatedPlayer.IP = p.Addr
		updatedPlayer.Id = p.Id

		err := UpdatePlayer(&player, &updatedPlayer)
		if err != nil {
			log.Printf("Failed to update player: %v", err)
			return err
		}

		log.Printf("Player updated: %v", player.GetPlayerName())

	}

	return nil
}
