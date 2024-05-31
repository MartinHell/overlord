package models

import (
	"fmt"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"gorm.io/gorm"
)

type Weapon struct {
	WeaponID  uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	Type      string
}

func (w *Weapon) FromCommonWeapon(r *common.Weapon) {
	if r == nil {
		return
	}

	if r.Type != "" {
		w.Type = r.Type
	}
}

func (w *Weapon) FindWeaponByType() error {

	result := initializers.DB.Where("type = ?", w.Type).First(&w)

	if result.Error != nil {
		logs.Sugar.Errorf("Failed to query weapon: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Weapon not found")
	}

	return nil

}

func (w *Weapon) CreateWeapon() error {
	// Query if weapon already exists
	if err := w.FindWeaponByType(); err.Error() == "Weapon not found" {
		// Create weapon
		result := initializers.DB.Create(w)

		if result.Error != nil {
			logs.Sugar.Errorf("Failed to create weapon: %v", result.Error)
			return result.Error
		}

		return nil
	} else {
		return nil
	}

}

func (w *Weapon) UpdateWeapon(uw *Weapon) error {

	hasChanges := false

	if w.Type != uw.Type {
		w.Type = uw.Type
		hasChanges = true
	}

	if hasChanges {
		result := initializers.DB.Model(&w).Updates(w)

		if result.Error != nil {
			logs.Sugar.Errorf("Failed to update weapon: %v", result.Error)
			return result.Error
		}
	}

	return nil
}

func ensureWeapon(tx *gorm.DB, weapon Weapon, weaponType string) (*uint, error) {
	if weapon.Type == "" {
		return nil, nil
	}

	var existingWeapon Weapon
	logs.Sugar.Debugf("Checking or creating %s with Type: %s", weaponType, weapon.Type)
	if err := tx.Where("type = ?", weapon.Type).FirstOrCreate(&existingWeapon, Weapon{Type: weapon.Type}).Error; err != nil {
		logs.Sugar.Errorf("Failed to find or create %s: %+v, error: %v", weaponType, existingWeapon, err)
		return nil, err
	}
	return &existingWeapon.WeaponID, nil
}
