package controllers

import (
	"log"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
)

/* func CreatePlayer(c *gin.Context) {
	// Get data off req body
	var player models.Player

	c.Bind(&player)

	// Validate data

	// Create player
	result := initializers.DB.Create(&player)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"player": player,
		})
		return
	}

	// Return player
	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

func GetPlayers(c *gin.Context) {
	var players []models.Player

	initializers.DB.Find(&players)

	c.JSON(http.StatusOK, gin.H{
		"players": players,
	})
}

func GetPlayer(c *gin.Context) {
	var player models.Player

	initializers.DB.First(&player, c.Param("id"))

	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

func UpdatePlayer(c *gin.Context) {
	// Find the player we're updating
	var player models.Player

	initializers.DB.First(&player, c.Param("id"))

	// Get data off req body
	var body models.Player
	c.Bind(&body)

	// Update the player
	initializers.DB.Model(&player).Updates(models.Player{
		Name: body.Name,
		UCID: body.UCID,
	})

	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

func DeletePlayer(c *gin.Context) {
	// Find the player we're deleting
	var player models.Player

	initializers.DB.First(&player, c.Param("id"))

	// Delete the player
	initializers.DB.Delete(&player)

	c.Status(http.StatusOK)
}

func GetPlayerByName(c *gin.Context) {
	var players []models.Player

	result := initializers.DB.Where("name LIKE ?", c.Param("id")).Find(&players)

	if result.Error != nil {
		log.Printf("Failed to query players: %v", result.Error)
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"players": players,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"players": players,
		})
	}
}

func GetPlayerByUCID(c *gin.Context) {
	var player models.Player

	resutl := initializers.DB.Where("uc_id = ?", c.Param("id")).First(&player)

	if resutl.Error != nil {
		log.Printf("Failed to query player: %v", resutl.Error)
	}

	if resutl.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"player": player,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"player": player,
		})
	}
}

func GetPlayerEvents(c *gin.Context) {
	var player models.Player

	result := initializers.DB.Preload("Events").First(&player, c.Param("id"))

	if result.Error != nil {
		log.Printf("Failed to query player events: %v", result.Error)
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"events": player.Events,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"events": player.Events,
		})
	}
}

func GetPlayerHits(c *gin.Context) {
	var hits int64

	result := initializers.DB.Model(&models.Event{}).
		Where("player_id = ? AND event = ?", c.Param("id"), "S_EVENT_HIT").
		Count(&hits)

	if result.Error != nil {
		log.Printf("Failed to query event count: %v", result.Error)
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"hits": hits,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"hits": hits,
		})
	}
} */

/* GRPC Work */

// CreatePlayer creates a player in the database
func CreatePlayer(p *models.Player) error {

	result := initializers.DB.Create(&p)
	if result.Error != nil {
		log.Printf("Failed to create player: %v", result.Error)
		return result.Error
	}

	return nil
}

// UpdatePlayer updates a player in the database
func UpdatePlayer(p *models.Player, up *models.Player) error {

	result := initializers.DB.Model(&p).Updates(up)
	if result.Error != nil {
		log.Printf("Failed to update player: %v", result.Error)
		return result.Error
	}

	return nil
}
