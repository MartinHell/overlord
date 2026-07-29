package models

import (
	"time"

	"github.com/MartinHell/overlord/logs"
	"gorm.io/gorm"
)

// What an event object turned out to be. Used for both the initiator and the
// target of an event, since DCS models them with the same oneof. Anything that
// was not a unit used to be discarded entirely, which lost the target on most
// air-to-ground kills and the initiator on roughly half of all hits.
const (
	ObjectKindUnit    = "unit"
	ObjectKindWeapon  = "weapon"
	ObjectKindStatic  = "static"
	ObjectKindScenery = "scenery"
	ObjectKindAirbase = "airbase"
	ObjectKindUnknown = "unknown"
)

type Target struct {
	TargetID  uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	PlayerID  uint
	WeaponID  uint
	UnitID    uint
	// Kind records which sort of thing was hit. Statics, scenery and airbases
	// store their type in Unit, so without this they would be indistinguishable
	// from aircraft.
	Kind   string `gorm:"index"`
	Player Player
	Unit   Unit
	Weapon Weapon
}

func ensureTarget(tx *gorm.DB, tgt Target) (*uint, error) {
	var target Target

	// A target is worth storing if we learned anything at all about it. The
	// previous check required a unit type, which silently dropped weapon,
	// static and scenery targets.
	if tgt.Unit.Type == "" && tgt.Weapon.Type == "" {
		return nil, nil
	}

	if tgt.Unit.Type != "" {
		unitID, err := ensureUnit(tx, tgt.Unit)
		if err != nil {
			return nil, err
		}
		target.UnitID = *unitID
	}

	if tgt.Weapon.Type != "" {
		weaponID, err := ensureWeapon(tx, tgt.Weapon, "Weapon")
		if err != nil {
			return nil, err
		}
		target.WeaponID = *weaponID
	}

	target.Kind = tgt.Kind
	if target.Kind == "" {
		target.Kind = ObjectKindUnknown
	}

	if err := tx.Where("unit_id = ? AND weapon_id = ? AND kind = ?", target.UnitID, target.WeaponID, target.Kind).
		FirstOrCreate(&target, target).Error; err != nil {
		logs.Sugar.Errorf("Failed to find or create target: %+v, error: %v", target, err)
		return nil, err
	}

	return &target.TargetID, nil
}
