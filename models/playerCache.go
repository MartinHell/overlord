package models

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
)

var errNoGrpcClient = errors.New("gRPC client is not configured")

// minRefreshInterval bounds how often a cache miss can trigger a fetch. Without
// it, every event mentioning someone who is not in the list would hit the DCS
// server again, which is the behaviour this cache exists to remove.
const minRefreshInterval = 5 * time.Second

// PlayerCache holds the server's player list. It exists so that resolving a
// player does not cost a gRPC round trip into the DCS sim thread on every
// single event, which is what the previous implementation did.
type PlayerCache struct {
	mu        sync.Mutex
	byName    map[string]*net.GetPlayersResponse_GetPlayerInfo
	attempted time.Time
}

// Players is the shared cache. Event handling is single-threaded today, but the
// GraphQL resolvers run concurrently, so it guards itself.
var Players = &PlayerCache{}

// FindPlayerByName resolves a player by name, fetching the list from DCS only
// when the cached copy is stale or the name is unknown.
func (p *PlayerCache) FindPlayerByName(name string) *net.GetPlayersResponse_GetPlayerInfo {
	if name == "" {
		return nil
	}

	// The synthetic AI players never appear in the server's player list, so
	// resolve them locally rather than fetching on every AI event.
	if IsAIPlayerName(name) {
		ai := AIPlayerFor(coalitionFromAIName(name))
		return &net.GetPlayersResponse_GetPlayerInfo{
			Id:   0,
			Name: ai.GetPlayerName(),
			Ucid: ai.UCID,
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// A hit is served as-is. Entries only go stale when the player list changes,
	// and connect and disconnect events keep the cache honest about that, so
	// there is no reason to expire a known-good entry on a timer.
	if player, ok := p.byName[name]; ok {
		return player
	}

	// Unknown name: they may have joined since the last fetch. Refresh at most
	// once per minRefreshInterval so a name that is never in the list, such as
	// an AI unit, cannot drive a fetch per event.
	if time.Since(p.attempted) >= minRefreshInterval {
		if err := p.refreshLocked(); err != nil {
			logs.Sugar.Errorf("Failed to refresh player cache: %v", err)
		}
	}

	return p.byName[name]
}

// Invalidate drops everything so the next lookup refetches. Called when the
// player list is known to have changed.
func (p *PlayerCache) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.byName = nil
	p.attempted = time.Time{}
}

// Add records a player the cache has not fetched yet. A connect event carries
// everything needed, so there is no reason to ask DCS for it again.
func (p *PlayerCache) Add(player *net.GetPlayersResponse_GetPlayerInfo) {
	if player == nil || player.GetName() == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.byName == nil {
		p.byName = map[string]*net.GetPlayersResponse_GetPlayerInfo{}
	}
	p.byName[player.GetName()] = player
}

func (p *PlayerCache) refreshLocked() error {
	p.attempted = time.Now()

	// Guard rather than panic: the gRPC client is nil until InitGrpc has run,
	// and a lookup should degrade to "unknown player" rather than crash.
	if initializers.NetServiceClient == nil {
		return errNoGrpcClient
	}

	logs.Sugar.Debug("Refreshing player cache")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := initializers.NetServiceClient.GetPlayers(ctx, &net.GetPlayersRequest{})
	if err != nil {
		return err
	}

	byName := make(map[string]*net.GetPlayersResponse_GetPlayerInfo, len(response.GetPlayers()))
	for _, player := range response.GetPlayers() {
		byName[player.GetName()] = player
	}

	p.byName = byName

	logs.Sugar.Debugf("Player cache refreshed: %d players", len(byName))

	return nil
}
