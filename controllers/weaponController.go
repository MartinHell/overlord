package controllers

import (
	"log"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/models"
)

/* func CreateWeapon(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{
		"weapon": weapon,
	})
}

func GetWeapons(c *gin.Context) {
	var weapons []models.Weapon

	initializers.DB.Find(&weapons)

	c.JSON(http.StatusOK, gin.H{
		"weapons": weapons,
	})
}

func GetWeapon(c *gin.Context) {
	var weapon models.Weapon

	initializers.DB.First(&weapon, c.Param("id"))

	c.JSON(http.StatusOK, gin.H{
		"weapon": weapon,
	})
}

func UpdateWeapon(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{
		"weapon": weapon,
	})
}

func DeleteWeapon(c *gin.Context) {
	// Find the weapon we're deleting
	var weapon models.Weapon

	initializers.DB.First(&weapon, c.Param("id"))

	// Delete weapon
	initializers.DB.Delete(&weapon)

	c.Status(http.StatusOK)
}

func GetWeaponByName(c *gin.Context) {
	var weapon models.Weapon

	result := initializers.DB.Where("name = ?", c.Param("id")).First(&weapon)

	if result.Error != nil {
		log.Printf("Failed to query players: %v", result.Error)
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"weapon": weapon,
		})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{
			"weapon": weapon,
		})
	}
}
*/

func FindWeaponByType(t string) (*models.Weapon, error) {
	var weapon models.Weapon

	result := initializers.DB.Where("type = ?", t).First(&weapon)

	if result.Error != nil {
		log.Printf("Failed to query weapon: %v", result.Error)
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &weapon, nil

}

func CreateWeapon(w *models.Weapon) error {
	// Create weapon
	result := initializers.DB.Create(w)

	if result.Error != nil {
		log.Printf("Failed to create weapon: %v", result.Error)
		return result.Error
	}

	return nil
}

func UpdateWeapon(w *models.Weapon, uw *models.Weapon) error {

	hasChanges := false

	if w.Type != uw.Type {
		w.Type = uw.Type
		hasChanges = true
	}

	if hasChanges {
		result := initializers.DB.Model(&w).Updates(w)

		if result.Error != nil {
			log.Printf("Failed to update weapon: %v", result.Error)
			return result.Error
		}
	}

	return nil
}
