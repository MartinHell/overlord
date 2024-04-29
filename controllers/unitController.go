package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

func UnitCreate(c *gin.Context) {
	// Get data off req body
	var unit models.Unit

	c.Bind(&unit)

	// Validate data

	// Create unit
	result := initializers.DB.Create(&unit)

	if result.Error != nil {
		c.Status(400)
		return
	}

	// Return unit
	c.JSON(200, gin.H{
		"player": unit,
	})
}

func UnitList(c *gin.Context) {
	var units []models.Unit

	initializers.DB.Find(&units)

	c.JSON(200, gin.H{
		"units": units,
	})
}

func UnitShow(c *gin.Context) {
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	c.JSON(200, gin.H{
		"unit": unit,
	})
}

func UnitUpdate(c *gin.Context) {
	// Find the unit we're updating
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	// Get data off req body
	var body models.Unit
	c.Bind(&body)

	// Update the unit
	result := initializers.DB.Model(&unit).Updates(
		models.Unit{
			Type:     body.Type,
			Category: body.Category,
		},
	)

	if result.Error != nil {
		c.Status(400)
		return
	}

	c.JSON(200, gin.H{
		"unit": unit,
	})
}

func UnitDelete(c *gin.Context) {
	// Find the unit we're deleting
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	// Delete the unit
	initializers.DB.Delete(&unit)

	c.Status(200)
}

func UnitSearchByName(c *gin.Context) {
	var units []models.Unit

	result := initializers.DB.Where("type = ?", c.Param("id")).Find(&units)

	if result.Error != nil {
		c.Status(400)
		return
	} else if result.RowsAffected == 0 {
		c.Status(404)
		return
	} else {
		c.JSON(200, gin.H{
			"units": units,
		})
	}
}
