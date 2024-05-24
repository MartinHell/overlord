package controllers

import (
	"net/http"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"github.com/gin-gonic/gin"
)

func ApiGetUnits(c *gin.Context) {
	var units []models.Unit

	initializers.DB.Find(&units)

	c.JSON(http.StatusOK, gin.H{
		"units": units,
	})
}

func ApiGetUnit(c *gin.Context) {
	var unit models.Unit

	initializers.DB.First(&unit, c.Param("id"))

	c.JSON(http.StatusOK, gin.H{
		"unit": unit,
	})
}

func ApiGetUnitByName(c *gin.Context) {
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
}

// CreateUnit creates a unit in the database
func CreateUnit(u *models.Unit) error {

	result := initializers.DB.Create(&u)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to create unit: %v", result.Error)
		return result.Error
	}

	return nil
}

// UpdatePlayer updates a unit in the database
func UpdateUnit(u *models.Unit, uu *models.Unit) error {

	hasChanges := false

	if u.Type != uu.Type {
		u.Type = uu.Type
		hasChanges = true
	}

	if hasChanges {
		result := initializers.DB.Model(&u).Updates(&u)
		if result.Error != nil {
			logs.Sugar.Errorf("Failed to update unit: %v", result.Error)
			return result.Error
		}
	}

	return nil
}

func GetUnit(u *models.Unit) (*models.Unit, error) {

	result := initializers.DB.Where("type = ?", u.Type).First(&u)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to get unit: %v", result.Error)
		return nil, result.Error
	}

	return u, nil
}
