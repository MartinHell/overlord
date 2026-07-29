package models

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
)

// AI players are synthetic and never appear in the server's player list, so
// resolving them must not reach for the network at all. The cache has no gRPC
// client configured under test, so a fetch would fail loudly.
func TestFindPlayerByNameResolvesAIWithoutFetching(t *testing.T) {
	cache := &PlayerCache{}

	for _, coalition := range []string{CoalitionRed, CoalitionBlue} {
		ai := AIPlayerFor(coalition)

		got := cache.FindPlayerByName(ai.GetPlayerName())
		if got == nil {
			t.Fatalf("expected the synthetic AI player for %s", coalition)
		}
		if got.GetUcid() != ai.UCID {
			t.Errorf("expected UCID %q, got %q", ai.UCID, got.GetUcid())
		}
	}
}

func TestFindPlayerByNameEmpty(t *testing.T) {
	cache := &PlayerCache{}

	if got := cache.FindPlayerByName(""); got != nil {
		t.Fatalf("expected nil for an empty name, got %+v", got)
	}
}

// A seeded entry is served without a fetch, which is the whole point: connect
// events carry everything the player list would tell us.
func TestAddThenFindServesFromCache(t *testing.T) {
	cache := &PlayerCache{}

	cache.Add(&net.GetPlayersResponse_GetPlayerInfo{
		Id:   4,
		Name: "Meekss",
		Ucid: "abc123",
	})

	got := cache.FindPlayerByName("Meekss")
	if got == nil {
		t.Fatal("expected the seeded player to be found")
	}
	if got.GetUcid() != "abc123" {
		t.Errorf("expected UCID abc123, got %q", got.GetUcid())
	}
}

func TestAddIgnoresUnusableEntries(t *testing.T) {
	cache := &PlayerCache{}

	cache.Add(nil)
	cache.Add(&net.GetPlayersResponse_GetPlayerInfo{Ucid: "no-name"})

	if len(cache.byName) != 0 {
		t.Fatalf("expected nothing to be cached, got %d entries", len(cache.byName))
	}
}
