package models

import (
	"errors"
	"strings"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"gorm.io/gorm"
)

type Player struct {
	PlayerID   uint `gorm:"unique;primaryKey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	PlayerName *string
	//Unit       `gorm:"foreignKey:UnitID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UCID string `gorm:"unique;not null"`
	//Id   uint32
	//UnitID     uint
}

// aiUCIDPrefix marks the synthetic players that stand in for AI units. Real
// UCIDs are 32-character hex strings, so this cannot collide with one.
const aiUCIDPrefix = "ai-"

// AIPlayerFor returns the synthetic player representing AI units of a given
// coalition. AI is tracked per coalition so that red and blue AI show up as
// separate players and their kills can be counted against each other.
func AIPlayerFor(coalition string) Player {
	if coalition == "" {
		coalition = CoalitionUnknown
	}

	name := "AI-Unit (" + coalition + ")"

	return Player{
		PlayerName: &name,
		UCID:       aiUCIDPrefix + coalition,
	}
}

// IsAIPlayerName reports whether a name belongs to one of the synthetic AI
// players rather than a human who might appear in the DCS player list.
func IsAIPlayerName(name string) bool {
	return strings.HasPrefix(name, "AI-Unit")
}

// coalitionFromAIName recovers the coalition from a synthetic AI player name,
// the inverse of the name built by AIPlayerFor.
func coalitionFromAIName(name string) string {
	open := strings.Index(name, "(")
	close := strings.LastIndex(name, ")")
	if open < 0 || close < open {
		return CoalitionUnknown
	}
	return name[open+1 : close]
}

// GetPlayerFromDB resolves the player's UCID from the DCS player list and, if
// they are already stored, fills in the rest of the record from the database.
// A player who cannot be resolved is left as-is rather than treated as an error:
// events routinely arrive after the player behind them has disconnected.
func (p *Player) GetPlayerFromDB() error {
	if err := p.GetPlayerUcidByName(); err != nil {
		return err
	}

	if p.UCID == "" {
		return nil
	}

	name := p.PlayerName

	var stored Player
	result := initializers.DB.Where(ucidQuery, p.UCID).First(&stored)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logs.Sugar.Errorf("Failed to find player: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return nil
	}

	*p = stored
	if p.PlayerName == nil {
		p.PlayerName = name
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

func (p *Player) GetPlayerByPlayerID(id uint) error {

	result := initializers.DB.Where("player_id = ?", id).First(&p)
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

	if hasChanges {
		result := initializers.DB.Model(&p).Where(ucidQuery, p.UCID).Updates(p)
		if result.Error != nil {
			logs.Sugar.Errorf("Failed to update player: %v", result.Error)
			return result.Error
		}
	}

	return nil
}

// EnsureInDB resolves the player's UCID and stores them if they are not known
// yet. Players whose UCID cannot be resolved are skipped rather than inserted
// with an empty UCID, which would collide on the unique index.
func (p *Player) EnsureInDB() error {
	if p.UCID == "" {
		if err := p.GetPlayerUcidByName(); err != nil {
			return err
		}
	}

	if p.UCID == "" {
		logs.Sugar.Debugf("Skipping player %q: no UCID available", p.GetPlayerName())
		return nil
	}

	var stored Player
	result := initializers.DB.Where(ucidQuery, p.UCID).First(&stored)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logs.Sugar.Errorf("Failed to find player: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected > 0 {
		p.PlayerID = stored.PlayerID
		return nil
	}

	return p.CreatePlayer()
}

// GetPlayerUcidByName fills in the player's UCID from the server's current
// player list. A player who is not in that list leaves the UCID empty; callers
// are expected to treat that as "not resolvable right now", not as an error.
func (p *Player) GetPlayerUcidByName() error {
	name := p.GetPlayerName()
	if name == "" {
		return nil
	}

	player := Players.FindPlayerByName(name)
	if player == nil {
		logs.Sugar.Debugf("Player %q is not in the DCS player list", name)
		return nil
	}

	p.UCID = player.GetUcid()

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
