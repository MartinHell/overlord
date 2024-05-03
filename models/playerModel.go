package models

import (
	"context"
	"fmt"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"gorm.io/gorm"
)

type PlayerCache struct {
	Players []*net.GetPlayersResponse_GetPlayerInfo
}

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

func (p *Player) GetPlayerUcid() string {
	playerInfo := net.GetPlayersResponse_GetPlayerInfo{
		Id:   p.GetId(),
		Name: p.GetPlayerName(),
	}

	tempPlayerInfo := &net.GetPlayersResponse_GetPlayerInfo{
		Ucid: playerInfo.Ucid,
		Name: playerInfo.Name,
	}

	fmt.Printf("%+v", tempPlayerInfo)

	return tempPlayerInfo.Ucid
	// Use tempPlayerInfo variable here
}

// GetPlayersCache returns a cache of players
/* func (p *PlayerCache) GetPlayersCache(netClient net.NetServiceClient) []*net.GetPlayersResponse_GetPlayerInfo {

	p, err := refreshPlayersCache(netClient)
	if err != nil {
		log.Printf("Failed to refresh players cache: %v", err)
	}

	return playersCache
} */

// This is a cache of players that we can use to avoid querying the database
func (p *PlayerCache) RefreshPlayersCache(netClient net.NetServiceClient) error {

	response, err := netClient.GetPlayers(context.Background(), &net.GetPlayersRequest{})
	if err != nil {
		return err
	}
	p.Players = response.Players
	return nil
}
