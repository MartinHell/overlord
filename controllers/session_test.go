package controllers

import (
	"testing"

	"github.com/MartinHell/overlord/models"
)

func newPlayer(name, ucid string) models.Player {
	return models.Player{PlayerName: &name, UCID: ucid}
}

// A disconnect event carries only an id, so resolving who left depends entirely
// on what was remembered at connect time.
func TestSessionRoundTrip(t *testing.T) {
	resetSessions()

	rememberSession(7, newPlayer("Meekss", "abc123"))

	player, ok := forgetSession(7)
	if !ok {
		t.Fatal("expected the session to be found")
	}
	if player.GetPlayerName() != "Meekss" || player.GetUCID() != "abc123" {
		t.Fatalf("wrong player returned: %+v", player)
	}

	// A second disconnect for the same id must not resolve again.
	if _, ok := forgetSession(7); ok {
		t.Fatal("expected the session to have been removed")
	}
}

func TestForgetUnknownSession(t *testing.T) {
	resetSessions()

	if _, ok := forgetSession(999); ok {
		t.Fatal("expected an unknown session id to report not found")
	}
}

// Overlord can start mid-mission, or the stream can drop and reconnect. Player
// ids are only unique within one connection, so stale sessions must not survive
// and be mistaken for a different player reusing the id.
func TestResetSessionsClearsState(t *testing.T) {
	resetSessions()

	rememberSession(1, newPlayer("Alpha", "aaa"))
	rememberSession(2, newPlayer("Bravo", "bbb"))

	resetSessions()

	if _, ok := forgetSession(1); ok {
		t.Fatal("expected sessions to be cleared")
	}
	if _, ok := forgetSession(2); ok {
		t.Fatal("expected sessions to be cleared")
	}
}

func TestRememberSessionOverwrites(t *testing.T) {
	resetSessions()

	rememberSession(3, newPlayer("Old", "old"))
	rememberSession(3, newPlayer("New", "new"))

	player, ok := forgetSession(3)
	if !ok {
		t.Fatal("expected the session to be found")
	}
	if player.GetUCID() != "new" {
		t.Fatalf("expected the id to be reassigned to the newer player, got %q", player.GetUCID())
	}
}
