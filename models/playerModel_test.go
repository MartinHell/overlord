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

func TestFindPlayerByNameResolvesAIPlayerPerCoalition(t *testing.T) {
	var cache PlayerCache

	for _, coalition := range []string{CoalitionRed, CoalitionBlue, CoalitionNeutral} {
		ai := AIPlayerFor(coalition)

		got := cache.FindPlayerByName(ai.GetPlayerName())
		if got == nil {
			t.Fatalf("expected the synthetic AI player for %s, got nil", coalition)
		}
		if got.GetUcid() != ai.UCID {
			t.Fatalf("expected UCID %q, got %q", ai.UCID, got.GetUcid())
		}
	}
}

func TestAIPlayersAreDistinctPerCoalition(t *testing.T) {
	red := AIPlayerFor(CoalitionRed)
	blue := AIPlayerFor(CoalitionBlue)

	if red.UCID == blue.UCID {
		t.Fatalf("red and blue AI must not share a UCID, both were %q", red.UCID)
	}
	if red.GetPlayerName() == blue.GetPlayerName() {
		t.Fatalf("red and blue AI must not share a name, both were %q", red.GetPlayerName())
	}
	if !IsAIPlayerName(red.GetPlayerName()) || !IsAIPlayerName(blue.GetPlayerName()) {
		t.Fatal("AI player names must be recognised as AI")
	}
	if IsAIPlayerName("Meekss") {
		t.Fatal("a human name must not be treated as AI")
	}
}

func TestAIPlayerForEmptyCoalitionFallsBack(t *testing.T) {
	ai := AIPlayerFor("")

	if ai.UCID != aiUCIDPrefix+CoalitionUnknown {
		t.Fatalf("expected the unknown-coalition UCID, got %q", ai.UCID)
	}
	if got := coalitionFromAIName(ai.GetPlayerName()); got != CoalitionUnknown {
		t.Fatalf("expected coalition %q back out of the name, got %q", CoalitionUnknown, got)
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
