package models

import "gorm.io/gorm"

type Weapon struct {
	gorm.Model
	Type   string
	Name   string  `gorm:"unique"`
	Events []Event `gorm:"foreignKey:WeaponID"`
}
