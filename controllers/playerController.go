package controllers

import (
	"net/http"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

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

func GetPlayerByName(c *gin.Context) {
	var players []models.Player

	result := initializers.DB.Where("name LIKE ?", c.Param("id")).Find(&players)

	if result.Error != nil {
		logs.Sugar.Errorf("Failed to query players: %v", result.Error)
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

	result := initializers.DB.Where("uc_id = ?", c.Param("id")).First(&player)

	if result.Error != nil {
		logs.Sugar.Errorf("Failed to query player: %v", result.Error)
	}

	if result.RowsAffected == 0 {
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

	var events []models.Event
	result := initializers.DB.Preload("Player").Preload("Initiator").Preload("Target").Preload("Weapon").Where("player_id = ?", c.Param("id")).Find(&events)

	if result.Error != nil {
		logs.Sugar.Errorf("Failed to query player events: %v", result.Error)
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"events": events,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"events": events,
		})
	}
}

// Implement this later
/* func GetPlayerHits(c *gin.Context) {
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
}
*/
