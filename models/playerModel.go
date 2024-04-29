package models

import (
	"time"

	"gorm.io/gorm"
)

type Player struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	Name      string
	UCID      string  `gorm:"unique;primaryKey"`
	Events    []Event `gorm:"foreignKey:PlayerID"`
}
