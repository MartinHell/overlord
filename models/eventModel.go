package models

import "gorm.io/gorm"

type Event struct {
	gorm.Model
	PlayerID uint   `json:"playerid"`
	Event    string `json:"event"`
	TargetID *uint  `json:"targetid"`
	WeaponID *uint  `json:"weaponid"`
}
