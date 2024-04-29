package models

import "gorm.io/gorm"

type Unit struct {
	gorm.Model
	Type     string `gorm:"unique"`
	Category string
	Events   []Event `gorm:"foreignKey:TargetID"`
}
