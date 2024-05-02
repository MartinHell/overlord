package models

import (
	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
)

type Unit struct {
	UnitID   uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	Id       uint32
	Name     string
	Callsign string
	common.Coalition
	Type          string
	Position      *Position     `gorm:"foreignKey:UnitID"`
	Group         *common.Group `gorm:"foreignKey:UnitID;references:Id"`
	NumberInGroup uint32
}

type Position struct {
	UnitID          uint `gorm:"primaryKey;index"`
	common.Position `gorm:"embedded"`
}
