package models

import (
	"context"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
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
		result := initializers.DB.Where(ucidQuery, playerLookup.GetUcid()).First(p)
		if result.Error != nil && result.Error.Error() != "record not found" {
			logs.Sugar.Errorf("Failed to find player: %v", result.Error)
			return result.Error
		}

		if result.RowsAffected == 0 {
			return nil
		}
	}

	return nil
}

func (p *Player) GetPlayerByUCID(ucid string) error {

	result := initializers.DB.Where(ucidQuery, ucid).First(&p)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to get player: %v", result.Error)
		return result.Error
	}

	return nil
}

// CreatePlayer creates a player in the database
func (p *Player) CreatePlayer() error {

	result := initializers.DB.Create(&p)
	if result.Error != nil {
		logs.Sugar.Errorf("Failed to create player: %v", result.Error)
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
		result := initializers.DB.Model(&p).Where(ucidQuery, p.UCID).Updates(p)
		if result.Error != nil {
			logs.Sugar.Errorf("Failed to update player: %v", result.Error)
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
		result := initializers.DB.Where(ucidQuery, playerLookup.GetUcid()).First(&player)
		if result.Error != nil && result.Error.Error() != "record not found" {
			logs.Sugar.Errorf("Failed to find player: %v", result.Error)
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
		logs.Sugar.Errorf("Failed to refresh player cache: %v", err)
	}

	player := *playercache.FindPlayerByName(p.GetPlayerName())

	p.UCID = player.Ucid
	return nil
}

func (p *Player) FromGetPlayersResponse_GetPlayerInfo(r *net.GetPlayersResponse_GetPlayerInfo) {
	p.PlayerName = &r.Name
	p.UCID = r.Ucid
}

func (p *Player) FromStreamEventsResponse_ConnectEvent(r *mission.StreamEventsResponse_ConnectEvent) {
	p.PlayerName = &r.Name
	p.UCID = r.Ucid
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

func (p *Player) GetPlayerUcid() string {
	var playercache PlayerCache
	err := playercache.RefreshPlayersCache()
	if err != nil {
		logs.Sugar.Errorf("Failed to refresh player cache: %v", err)
	}

	player := *playercache.FindPlayerByName(p.GetPlayerName())

	return player.Ucid
}

// Find Player in cache based on Name
func (p *PlayerCache) FindPlayerByName(name string) *net.GetPlayersResponse_GetPlayerInfo {

	if name == *AIPlayer.PlayerName {
		player := &net.GetPlayersResponse_GetPlayerInfo{
			Id:   0,
			Name: *AIPlayer.PlayerName,
			Ucid: AIPlayer.UCID,
		}
		return player
	}
	logs.Sugar.Debugf("Finding player by name: %s", name)
	for _, player := range p.Players {
		if player.Name == name {
			return player
		}
	}
	return nil
}

// Find Player in cache based on UCID
func (p *PlayerCache) FindPlayerByUCID(ucid string) *net.GetPlayersResponse_GetPlayerInfo {

	logs.Sugar.Debugf("Finding player by UCID: %s", ucid)
	for _, player := range p.Players {
		if player.Ucid == ucid {
			return player
		}
	}
	return nil
}

// This is a cache of players that we can use to avoid querying the database
func (p *PlayerCache) RefreshPlayersCache() error {

	logs.Sugar.Debug("Refreshing player cache")
	response, err := initializers.NetServiceClient.GetPlayers(context.Background(), &net.GetPlayersRequest{})
	if err != nil {
		return err
	}

	logs.Sugar.Debugf("%+v", response)
	p.Players = response.Players
	return nil
}

func ensurePlayer(tx *gorm.DB, player Player) (*uint, error) {
	if player.UCID == "" {
		return nil, nil
	}

	var existingPlayer Player
	logs.Sugar.Debugf("Checking or creating Player with UCID: %s", player.UCID)
	if err := tx.Where("uc_id = ?", player.UCID).FirstOrCreate(&existingPlayer, Player{UCID: player.UCID, PlayerName: player.PlayerName}).Error; err != nil {
		logs.Sugar.Errorf("Failed to find or create Player: %+v, error: %v", existingPlayer, err)
		return nil, err
	}
	return &existingPlayer.PlayerID, nil
}
