package models

import (
	"context"
	"log"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/initializers"
	"gorm.io/gorm"
)

type PlayerCache struct {
	Players []*net.GetPlayersResponse_GetPlayerInfo
}

type Player struct {
	PlayerID   uint `gorm:"unique;primaryKey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	PlayerName *string
	//Unit       `gorm:"foreignKey:UnitID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UCID string `gorm:"unique;not null"`
	IP   string
	//Id   uint32
	//UnitID     uint
}

var aiPlayerName = "AI-Unit"

var AIPlayer = Player{
	PlayerName: &aiPlayerName,
	UCID:       "0",
}

func (p *Player) GetPlayerFromDB() error {
	// Check if player is in DB
	var playerCache PlayerCache

	playerCache.RefreshPlayersCache()
	playerLookup := *playerCache.FindPlayerByName(*p.PlayerName)

	if playerLookup.GetUcid() != "" {
		result := initializers.DB.Where("uc_id = ?", playerLookup.GetUcid()).First(p)
		if result.Error != nil && result.Error.Error() != "record not found" {
			log.Printf("Failed to find player: %v", result.Error)
			return result.Error
		}

		if result.RowsAffected == 0 {
			return nil
		}
	}

	return nil
}

func (p *Player) GetPlayerByUCID(ucid string) error {

	result := initializers.DB.Where("uc_id = ?", ucid).First(&p)
	if result.Error != nil {
		log.Printf("Failed to get player: %v", result.Error)
		return result.Error
	}

	return nil
}

// CreatePlayer creates a player in the database
func (p *Player) CreatePlayer() error {

	result := initializers.DB.Create(&p)
	if result.Error != nil {
		log.Printf("Failed to create player: %v", result.Error)
		return result.Error
	}

	return nil
}

// UpdatePlayer updates a player in the database
func (p *Player) UpdatePlayer(up *Player) error {

	hasChanges := false

	if up.PlayerName != nil {
		p.PlayerName = up.PlayerName
		hasChanges = true
	}
	if up.UCID != "" {
		p.UCID = up.UCID
		hasChanges = true
	}
	if up.IP != "" {
		p.IP = up.IP
		hasChanges = true
	} /*
		if up.Id != 0 {
			p.Id = up.Id
			hasChanges = true
		} */

	if hasChanges {
		result := initializers.DB.Model(&p).Where("uc_id = ?", p.UCID).Updates(p)
		if result.Error != nil {
			log.Printf("Failed to update player: %v", result.Error)
			return result.Error
		}
	}

	return nil
}

func (p *Player) CheckIfPlayerInDB() bool {
	// Check if player is in DB
	var playerCache PlayerCache
	var player Player

	playerCache.RefreshPlayersCache()
	playerLookup := *playerCache.FindPlayerByName(*p.PlayerName)

	if playerLookup.GetUcid() != "" {
		result := initializers.DB.Where("uc_id = ?", playerLookup.GetUcid()).First(&player)
		if result.Error != nil && result.Error.Error() != "record not found" {
			log.Printf("Failed to find player: %v", result.Error)
		}

		if result.RowsAffected == 0 {
			return false
		}
	}

	return true

}

func (p *Player) GetPlayerUcidByName() error {
	var playercache PlayerCache
	err := playercache.RefreshPlayersCache()
	if err != nil {
		log.Panicf("Failed to refresh player cache: %v", err)
	}

	player := *playercache.FindPlayerByName(p.GetPlayerName())

	p.UCID = player.Ucid
	return nil
}

func (p *Player) FromGetPlayersResponse_GetPlayerInfo(r *net.GetPlayersResponse_GetPlayerInfo) {
	/* p.Id = r.Id */
	p.PlayerName = &r.Name
	p.UCID = r.Ucid
	//p.IP = r.RemoteAddress
}

func (p *Player) FromStreamEventsResponse_ConnectEvent(r *mission.StreamEventsResponse_ConnectEvent) {
	p.PlayerName = &r.Name
	p.UCID = r.Ucid
	//p.IP = r.Addr
	/* p.Id = r.Id */
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

/*
	 func (p *Player) GetId() uint32 {
		if p != nil {
			return p.Id
		}
		return 0
	}
*/
func (p *Player) GetPlayerUcid() string {
	var playercache PlayerCache
	err := playercache.RefreshPlayersCache()
	if err != nil {
		log.Panicf("Failed to refresh player cache: %v", err)
	}

	player := *playercache.FindPlayerByName(p.GetPlayerName())

	return player.Ucid
}

// Find Player in cache based on Name
func (p *PlayerCache) FindPlayerByName(name string) *net.GetPlayersResponse_GetPlayerInfo {

	if name == "AI-Unit" {
		player := &net.GetPlayersResponse_GetPlayerInfo{
			Id:   0,
			Name: "AI-Unit",
			Ucid: "0",
		}
		return player
	}
	log.Println("Finding player by name: ", name)
	for _, player := range p.Players {
		if player.Name == name {
			return player
		}
	}
	return nil
}

// Find Player in cache based on UCID
func (p *PlayerCache) FindPlayerByUCID(ucid string) *net.GetPlayersResponse_GetPlayerInfo {

	log.Println("Finding player by UCID: ", ucid)
	for _, player := range p.Players {
		if player.Ucid == ucid {
			return player
		}
	}
	return nil
}

// This is a cache of players that we can use to avoid querying the database
func (p *PlayerCache) RefreshPlayersCache() error {

	log.Print("Refreshing player cache")
	response, err := initializers.NetServiceClient.GetPlayers(context.Background(), &net.GetPlayersRequest{})
	if err != nil {
		return err
	}

	log.Printf("%+v", response)
	p.Players = response.Players
	return nil
}
