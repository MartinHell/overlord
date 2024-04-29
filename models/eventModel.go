package models

import "gorm.io/gorm"

type Event struct {
	gorm.Model
	PlayerID uint
	Event    string
	TargetID *uint
	WeaponID *uint
}
