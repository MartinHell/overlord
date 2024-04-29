package models

import "gorm.io/gorm"

type Event struct {
	gorm.Model
	PlayerID uint   `json:"PlayerID"`
	Event    string `json:"Event"`
	TargetID *uint  `json:"TargetID"`
	WeaponID *uint  `json:"WeaponID"`
}
