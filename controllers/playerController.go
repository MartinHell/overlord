package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

func PlayerCreate(c *gin.Context) {
	// Get data off req body
	var player models.Player

	c.Bind(&player)

	// Validate data

	// Create player
	result := initializers.DB.Create(&player)

	if result.Error != nil {
		c.Status(400)
		return
	}

	// Return player
	c.JSON(200, gin.H{
		"player": player,
	})
}

func PlayerList(c *gin.Context) {
	var players []models.Player

	initializers.DB.Find(&players)

	c.JSON(200, gin.H{
		"players": players,
	})
}

func PlayerShow(c *gin.Context) {
	var player models.Player

	initializers.DB.First(&player, c.Param("id"))

	c.JSON(200, gin.H{
		"player": player,
	})
}

func PlayerUpdate(c *gin.Context) {
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

	c.JSON(200, gin.H{
		"player": player,
	})
}

func PlayerDelete(c *gin.Context) {
	// Find the player we're deleting
	var player models.Player

	initializers.DB.First(&player, c.Param("id"))

	// Delete the player
	initializers.DB.Delete(&player)

	c.Status(200)
}

func PlayerSearch(c *gin.Context) {
	var players []models.Player

	if c.Query("name") != "" {
		initializers.DB.Where("LOWER(name) LIKE ?", "%"+c.Query("name")+"%").Find(&players)
	} else if c.Query("ucid") != "" {
		initializers.DB.Where("ucid = ?", c.Query("ucid")).Find(&players)
	}

	c.JSON(200, gin.H{
		"players": players,
	})
}

func PlayerEvents(c *gin.Context) {
	var player models.Player

	initializers.DB.Preload("Events").First(&player, c.Param("id"))

	c.JSON(200, gin.H{
		"events": player.Events,
	})
}
