package controllers

import (
	"sync"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// A disconnect event carries only the server-assigned id and a reason, with no
// name or UCID, so the only way to tell who left is to remember who arrived.
// Connect events carry everything needed, so this is populated from them rather
// than by asking DCS.
var (
	sessionsMu sync.Mutex
	sessions   = map[uint32]models.Player{}
)

// ConnectEvent handles the connect event
func ConnectEvent(p *mission.StreamEventsResponse_ConnectEvent) error {
	var connectedPlayer models.Player

	connectedPlayer.FromStreamEventsResponse_ConnectEvent(p)
	connectedPlayer.IP = p.GetAddr()

	// The connect event carries the UCID directly, so the player can be stored
	// without consulting the server's player list.
	if err := connectedPlayer.EnsureInDB(); err != nil {
		logs.Sugar.Errorf(logCreatePlayer, err)
		return err
	}

	// Reconnecting under a new name or from a new address is common, so refresh
	// the stored details.
	updated := models.Player{
		UCID:       p.GetUcid(),
		PlayerName: &p.Name,
		IP:         p.GetAddr(),
	}

	if err := connectedPlayer.UpdatePlayer(&updated); err != nil {
		logs.Sugar.Errorf("Failed to update player: %v", err)
		return err
	}

	rememberSession(p.GetId(), connectedPlayer)

	// Seed rather than invalidate: the connect event already tells us everything
	// the player list would, so there is no need to fetch it again.
	models.Players.Add(&net.GetPlayersResponse_GetPlayerInfo{
		Id:            p.GetId(),
		Name:          p.GetName(),
		Ucid:          p.GetUcid(),
		RemoteAddress: p.GetAddr(),
	})

	logs.Sugar.Infof("Player connected: %s (%s)", connectedPlayer.GetPlayerName(), p.GetUcid())

	return nil
}

// DisconnectEvent handles a player leaving.
func DisconnectEvent(p *mission.StreamEventsResponse_DisconnectEvent) error {
	player, known := forgetSession(p.GetId())

	// The player is gone from the server list, so the cached copy is stale.
	models.Players.Invalidate()

	if !known {
		// Overlord may have started mid-mission and never seen the connect.
		logs.Sugar.Debugf("Disconnect for unknown session id %d (reason: %v)", p.GetId(), p.GetReason())
		return nil
	}

	logs.Sugar.Infof("Player disconnected: %s (%s), reason: %v",
		player.GetPlayerName(), player.GetUCID(), p.GetReason())

	return nil
}

func rememberSession(id uint32, player models.Player) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	sessions[id] = player
}

func forgetSession(id uint32) (models.Player, bool) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	player, ok := sessions[id]
	delete(sessions, id)

	return player, ok
}

// resetSessions drops all tracked sessions. The stream reconnects after a
// mission restart, at which point every previously connected player is gone and
// their ids can be reused.
func resetSessions() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if len(sessions) > 0 {
		logs.Sugar.Debugf("Clearing %d tracked player sessions", len(sessions))
	}

	sessions = map[uint32]models.Player{}
}
