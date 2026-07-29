package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// CreateUnit creates a unit in the database
func CreateUnit(u *models.Unit) error {
	result := initializers.DB.Create(&u)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to create unit: %v", result.Error)
		return result.Error
	}

	return nil
}

// UpdateUnit updates a unit in the database
func UpdateUnit(u *models.Unit, uu *models.Unit) error {
	if u.Type == uu.Type {
		return nil
	}

	u.Type = uu.Type

	result := initializers.DB.Model(&u).Updates(&u)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to update unit: %v", result.Error)
		return result.Error
	}

	return nil
}
