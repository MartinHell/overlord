package models

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
)

func TestFindPlayerByNameMissingReturnsNil(t *testing.T) {
	// This used to be dereferenced unconditionally by the event handlers, which
	// panicked whenever an event arrived for a player who had already left.
	cache := PlayerCache{Players: []*net.GetPlayersResponse_GetPlayerInfo{
		{Id: 1, Name: "Viper", Ucid: "abc"},
	}}

	if got := cache.FindPlayerByName("Ghost"); got != nil {
		t.Fatalf("expected nil for an unknown player, got %+v", got)
	}
}

func TestFindPlayerByNameFindsPlayer(t *testing.T) {
	cache := PlayerCache{Players: []*net.GetPlayersResponse_GetPlayerInfo{
		{Id: 1, Name: "Viper", Ucid: "abc"},
		{Id: 2, Name: "Ghost", Ucid: "def"},
	}}

	got := cache.FindPlayerByName("Ghost")
	if got == nil {
		t.Fatal("expected to find Ghost, got nil")
	}
	if got.GetUcid() != "def" {
		t.Fatalf("expected UCID def, got %q", got.GetUcid())
	}
}

func TestFindPlayerByNameResolvesAIPlayer(t *testing.T) {
	var cache PlayerCache

	got := cache.FindPlayerByName(*AIPlayer.PlayerName)
	if got == nil {
		t.Fatal("expected the synthetic AI player, got nil")
	}
	if got.GetUcid() != AIPlayer.UCID {
		t.Fatalf("expected UCID %q, got %q", AIPlayer.UCID, got.GetUcid())
	}
}

func TestGetPlayerNameHandlesNil(t *testing.T) {
	var player *Player
	if got := player.GetPlayerName(); got != "" {
		t.Fatalf("expected an empty name for a nil player, got %q", got)
	}

	empty := &Player{}
	if got := empty.GetPlayerName(); got != "" {
		t.Fatalf("expected an empty name when PlayerName is nil, got %q", got)
	}
}
