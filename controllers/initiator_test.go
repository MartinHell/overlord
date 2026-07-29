package controllers

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/MartinHell/overlord/models"
)

// A static SAM site firing used to produce an event with an empty initiator,
// silently attributed to the AI player.
func TestBuildInitiatorStatic(t *testing.T) {
	initiator := &common.Initiator{Initiator: &common.Initiator_Static{Static: &common.Static{
		Type:      "SAM SA-2 LN",
		Coalition: common.Coalition_COALITION_RED,
	}}}

	got := buildInitiator(initiator)

	if got.Kind != models.ObjectKindStatic {
		t.Errorf("expected kind %q, got %q", models.ObjectKindStatic, got.Kind)
	}
	if got.Unit.Type != "SAM SA-2 LN" {
		t.Errorf("expected the static type to be recorded, got %q", got.Unit.Type)
	}
	if got.Coalition != models.CoalitionRed {
		t.Errorf("expected red, got %q", got.Coalition)
	}
	if got.Player.GetPlayerName() != "AI-Unit (red)" {
		t.Errorf("expected the red AI player, got %q", got.Player.GetPlayerName())
	}
}

func TestBuildInitiatorWeapon(t *testing.T) {
	initiator := &common.Initiator{Initiator: &common.Initiator_Weapon{Weapon: &common.Weapon{
		Type: "AIM_120C",
	}}}

	got := buildInitiator(initiator)

	if got.Kind != models.ObjectKindWeapon {
		t.Errorf("expected kind %q, got %q", models.ObjectKindWeapon, got.Kind)
	}
	if got.Unit.Type != "AIM_120C" {
		t.Errorf("expected the weapon type to be recorded, got %q", got.Unit.Type)
	}
}

func TestBuildInitiatorScenery(t *testing.T) {
	initiator := &common.Initiator{Initiator: &common.Initiator_Scenery{Scenery: &common.Scenery{
		Type: "Bridge",
	}}}

	got := buildInitiator(initiator)

	if got.Kind != models.ObjectKindScenery {
		t.Errorf("expected kind %q, got %q", models.ObjectKindScenery, got.Kind)
	}
	if got.Unit.Type != "Bridge" {
		t.Errorf("expected Bridge, got %q", got.Unit.Type)
	}
}

func TestBuildInitiatorUnitKeepsCoalition(t *testing.T) {
	initiator := &common.Initiator{Initiator: &common.Initiator_Unit{Unit: &common.Unit{
		Type:      "Su-27",
		Coalition: common.Coalition_COALITION_RED,
	}}}

	got := buildInitiator(initiator)

	if got.Kind != models.ObjectKindUnit {
		t.Errorf("expected kind %q, got %q", models.ObjectKindUnit, got.Kind)
	}
	if got.Unit.Type != "Su-27" {
		t.Errorf("expected Su-27, got %q", got.Unit.Type)
	}
	if got.Coalition != models.CoalitionRed {
		t.Errorf("expected red, got %q", got.Coalition)
	}
}

// DCS leaves the initiator unset for some events, such as flying an aircraft
// into a building. That must not panic, and must not masquerade as a real unit.
func TestBuildInitiatorNil(t *testing.T) {
	got := buildInitiator(nil)

	if got.Kind != models.ObjectKindUnknown {
		t.Errorf("expected kind %q, got %q", models.ObjectKindUnknown, got.Kind)
	}
	if got.Unit.Type != "" {
		t.Errorf("expected no unit type, got %q", got.Unit.Type)
	}
	if got.Coalition != models.CoalitionUnknown {
		t.Errorf("expected unknown coalition, got %q", got.Coalition)
	}
}
