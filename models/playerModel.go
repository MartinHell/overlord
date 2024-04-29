package models

import (
	"gorm.io/gorm"
)

type Player struct {
	gorm.Model
	Name   string
	UCID   string  `gorm:"unique"`
	Events []Event `gorm:"foreignKey:PlayerID"`
}
