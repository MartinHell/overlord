package models

import (
	"time"

	"github.com/MartinHell/overlord/logs"
	"gorm.io/gorm"
)

type Target struct {
	TargetID  uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	PlayerID  uint
	WeaponID  uint
	UnitID    uint
	Player    Player
	Unit      Unit
	Weapon    Weapon
}

func ensureTarget(tx *gorm.DB, tgt Target) (*uint, error) {
	var target Target

	if tgt.Unit.Type == "" {
		return nil, nil
	}

	unitID, err := ensureUnit(tx, tgt.Unit)
	if err != nil {
		return nil, err
	}

	target.UnitID = *unitID

	if tgt.Weapon.Type != "" {
		weaponID, err := ensureWeapon(tx, tgt.Weapon, "Weapon")
		if err != nil {
			return nil, err
		}
		target.WeaponID = *weaponID
	}

	if err := tx.Where("unit_id = ? AND weapon_id = ?", target.UnitID, target.WeaponID).FirstOrCreate(&target, target).Error; err != nil {
		logs.Sugar.Errorf("Failed to find or create target: %+v, error: %v", target, err)
		return nil, err
	}

	return &target.TargetID, nil
}
