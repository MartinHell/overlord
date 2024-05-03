package controllers

import (
	"log"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
)

/* func CreateUnit(c *gin.Context) {
	// Get data off req body
	var unit models.Unit

	c.Bind(&unit)

	// Validate data

	// Create unit
	result := initializers.DB.Create(&unit)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"unit": unit,
		})
		return
	}

	// Return unit
	c.JSON(http.StatusOK, gin.H{
		"player": unit,
	})
}

func GetUnits(c *gin.Context) {
	var units []models.Unit

	initializers.DB.Find(&units)

	c.JSON(http.StatusOK, gin.H{
		"units": units,
	})
}

func GetUnit(c *gin.Context) {
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	c.JSON(http.StatusOK, gin.H{
		"unit": unit,
	})
}

func UpdateUnit(c *gin.Context) {
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

	c.JSON(http.StatusOK, gin.H{
		"unit": unit,
	})
}

func DeleteUnit(c *gin.Context) {
	// Find the unit we're deleting
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	// Delete the unit
	initializers.DB.Delete(&unit)

	c.Status(http.StatusOK)
}

func GetUnitByName(c *gin.Context) {
	var units []models.Unit

	result := initializers.DB.Where("type = ?", c.Param("id")).Find(&units)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"units": units,
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"units": units,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"units": units,
		})
	}
} */

// CreateUnit creates a unit in the database
func CreateUnit(u *models.Unit) error {

	result := initializers.DB.Create(&u)
	if result.Error != nil {
		log.Printf("Failed to create unit: %v", result.Error)
		return result.Error
	}

	return nil
}

// UpdatePlayer updates a unit in the database
func UpdateUnit(u *models.Unit, uu *models.Unit) error {

	result := initializers.DB.Model(&u).Updates(uu)
	if result.Error != nil {
		log.Printf("Failed to update player: %v", result.Error)
		return result.Error
	}

	return nil
}
