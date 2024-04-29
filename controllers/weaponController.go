package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

func WeaponCreate(c *gin.Context) {
	// Get data off req body
	var weapon models.Weapon

	c.Bind(&weapon)

	// Validate data

	// Create weapon
	result := initializers.DB.Create(&weapon)

	if result.Error != nil {
		c.Status(400)
		return
	}

	// Return weapon
	c.JSON(200, gin.H{
		"weapon": weapon,
	})
}

func WeaponList(c *gin.Context) {
	var weapons []models.Weapon

	initializers.DB.Find(&weapons)

	c.JSON(200, gin.H{
		"weapons": weapons,
	})
}

func WeaponShow(c *gin.Context) {
	var weapon models.Weapon

	initializers.DB.First(&weapon, c.Param("id"))

	c.JSON(200, gin.H{
		"weapon": weapon,
	})
}

func WeaponUpdate(c *gin.Context) {
	// Find the weapon we're updating
	var weapon models.Weapon

	initializers.DB.First(&weapon, c.Param("id"))

	// Get data off req body
	var body models.Weapon
	c.Bind(&body)

	// Update weapon
	result := initializers.DB.Model(&weapon).Updates(body)

	if result.Error != nil {
		c.Status(400)
		return
	}

	// Return weapon
	c.JSON(200, gin.H{
		"weapon": weapon,
	})
}

func WeaponDelete(c *gin.Context) {
	// Find the weapon we're deleting
	var weapon models.Weapon

	initializers.DB.First(&weapon, c.Param("id"))

	// Delete weapon
	initializers.DB.Delete(&weapon)

	c.Status(200)
}
