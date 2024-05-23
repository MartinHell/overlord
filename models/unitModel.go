package models

import (
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"gorm.io/gorm"
)

type Unit struct {
	UnitID    uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	//Id        uint32
	Type string `gorm:"not null;unique;index"`
	/*
		Name      string
		Callsign  string
		common.Coalition
		Position      *common.Position `gorm:"embedded"`
		Group         *common.Group `gorm:"foreignKey:Id;references:Id"`
		NumberInGroup uint32
	*/
}

func (u *Unit) FromCommonUnit(r *common.Unit) {
	//u.Id = r.Id
	u.Type = r.Type
	/*
		u.Name = r.Name
		u.Callsign = r.Callsign
		u.Coalition = r.Coalition
		u.Position = &common.Position{
			Lat: r.Position.GetLat(),
			Lon: r.Position.GetLon(),
			Alt: r.Position.GetAlt(),
		}
		u.Group = &common.Group{
			Name: r.Group.GetName(),
			Id:   r.Group.GetId(),
		}
		u.NumberInGroup = r.NumberInGroup
	*/
}
