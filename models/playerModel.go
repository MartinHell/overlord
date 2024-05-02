package models

import (
	"time"

	"gorm.io/gorm"
)

type Player struct {
	PlayerID   uint32 `gorm:"unique;primaryKey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	PlayerName *string
	Unit       `gorm:"foreignKey:UnitID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UCID       string `gorm:"unique;not null"`
	IP         string
	Id         uint32
	UnitID     uint
}

func (p *Player) GetPlayerName() string {
	if p != nil && p.PlayerName != nil {
		return *p.PlayerName
	}
	return ""
}

func (p *Player) GetUCID() string {
	if p != nil {
		return p.UCID
	}
	return ""
}

func (p *Player) GetIP() string {
	if p != nil {
		return p.IP
	}
	return ""
}

func (p *Player) GetId() uint32 {
	if p != nil {
		return p.Id
	}
	return 0
}
