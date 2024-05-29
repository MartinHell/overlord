package models

import (
	"time"

	"gorm.io/gorm"
)

type Target struct {
	TargetID  uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	WeaponID  uint
	UnitID    uint
	Unit      Unit
	Weapon    Weapon
}
