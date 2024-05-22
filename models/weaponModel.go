package models

import (
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
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
	w.Type = r.Type
}
