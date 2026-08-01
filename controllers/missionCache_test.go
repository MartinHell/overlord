package controllers

import (
	"testing"
	"time"

	"github.com/MartinHell/overlord/models"
)

// seedMissionCache fills the cache as a completed load would have, and puts it
// back afterwards so the tests do not leak into each other.
func seedMissionCache(t *testing.T, rows []models.MissionSummary) {
	t.Helper()

	missionsCache.mu.Lock()
	missionsCache.rows = rows
	missionsCache.at = time.Now()
	missionsCache.mu.Unlock()

	t.Cleanup(invalidateMissions)
}

// A warm cache must not touch the database. initializers.DB is nil in this test
// binary, so a query would panic -- which is the assertion.
func TestGetMissionsServesFromCache(t *testing.T) {
	seedMissionCache(t, []models.MissionSummary{
		{MissionID: 2, Name: "Op Foothold", Theatre: "Caucasus", Events: 400},
		{MissionID: 1, Name: "Op Icebreaker", Theatre: "Syria", Events: 250},
	})

	got, err := GetMissions()
	if err != nil {
		t.Fatalf("expected a cached list, got error: %v", err)
	}
	if len(got) != 2 || got[0].MissionID != 2 || got[0].Name != "Op Foothold" {
		t.Fatalf("expected the cached rows in order, got %+v", got)
	}
}

// Callers get a copy. Badges, sorties and the mission index all take this list
// and build on it; one of them sorting or rewriting it must not change what the
// next caller sees.
func TestGetMissionsHandsOutIsolatedCopies(t *testing.T) {
	seedMissionCache(t, []models.MissionSummary{
		{MissionID: 1, Name: "Op Icebreaker", Events: 250},
	})

	first, err := GetMissions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first[0].Name = "clobbered"
	first[0].Events = 0

	second, err := GetMissions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second[0].Name != "Op Icebreaker" || second[0].Events != 250 {
		t.Fatalf("a caller's edits reached the cache: %+v", second[0])
	}
}

// A mission starting, or being named, has to be visible before the TTL is up.
func TestInvalidateMissionsForcesAReload(t *testing.T) {
	seedMissionCache(t, []models.MissionSummary{{MissionID: 1}})

	invalidateMissions()

	missionsCache.mu.Lock()
	stale := missionsCache.at.IsZero()
	missionsCache.mu.Unlock()

	if !stale {
		t.Fatal("expected invalidation to mark the cache for reload")
	}
}

// The TTL has to be under the dashboard's poll interval, or tabs polling
// together stop overlapping inside one window and the cache buys nothing.
func TestMissionsTTLIsShorterThanTheDashboardPoll(t *testing.T) {
	const dashboardPoll = 15 * time.Second

	if missionsTTL >= dashboardPoll {
		t.Fatalf("missionsTTL %s must be under the %s poll", missionsTTL, dashboardPoll)
	}
}
