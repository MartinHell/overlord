package controllers

import (
	"errors"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"gorm.io/gorm"
)

// Lookups backing the GraphQL query resolvers. A missing row is not an error:
// the schema returns a nullable object, so callers get null rather than an
// error payload.

func GetPlayers() ([]*models.Player, error) {
	var players []*models.Player

	if err := initializers.DB.Order("player_id").Find(&players).Error; err != nil {
		logs.Sugar.Errorf("Failed to list players: %v", err)
		return nil, err
	}

	return players, nil
}

func GetPlayerByID(id string) (*models.Player, error) {
	return firstOrNil[models.Player](initializers.DB.Where("player_id = ?", id))
}

func GetUnits() ([]*models.Unit, error) {
	var units []*models.Unit

	if err := initializers.DB.Order("unit_id").Find(&units).Error; err != nil {
		logs.Sugar.Errorf("Failed to list units: %v", err)
		return nil, err
	}

	return units, nil
}

func GetUnitByID(id string) (*models.Unit, error) {
	return firstOrNil[models.Unit](initializers.DB.Where("unit_id = ?", id))
}

func GetWeapons() ([]*models.Weapon, error) {
	var weapons []*models.Weapon

	if err := initializers.DB.Order("weapon_id").Find(&weapons).Error; err != nil {
		logs.Sugar.Errorf("Failed to list weapons: %v", err)
		return nil, err
	}

	return weapons, nil
}

func GetWeaponByID(id string) (*models.Weapon, error) {
	return firstOrNil[models.Weapon](initializers.DB.Where("weapon_id = ?", id))
}

// firstOrNil returns the first matching row, or nil when there is none. Only a
// real database failure is reported as an error.
func firstOrNil[T any](query *gorm.DB) (*T, error) {
	var row T

	err := query.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		logs.Sugar.Errorf("Lookup failed: %v", err)
		return nil, err
	}

	return &row, nil
}
