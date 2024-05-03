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
	Position      *common.Position `gorm:"embedded"`
	Group         *common.Group    `gorm:"foreignKey:Id;references:Id"`
	NumberInGroup uint32
}
