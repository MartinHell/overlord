package controllers

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/MartinHell/overlord/models"
)

func TestBuildTargetUnit(t *testing.T) {
	protoTarget := &common.Target{Target: &common.Target_Unit{Unit: &common.Unit{
		Type:      "Su-27",
		Coalition: common.Coalition_COALITION_RED,
	}}}

	target, coalition := buildTarget(protoTarget)

	if target.Kind != models.ObjectKindUnit {
		t.Errorf("expected kind %q, got %q", models.ObjectKindUnit, target.Kind)
	}
	if target.Unit.Type != "Su-27" {
		t.Errorf("expected unit type Su-27, got %q", target.Unit.Type)
	}
	if coalition != models.CoalitionRed {
		t.Errorf("expected red, got %q", coalition)
	}
}

// Weapon targets were parsed and then silently thrown away by ensureTarget,
// which required a unit type.
func TestBuildTargetWeapon(t *testing.T) {
	protoTarget := &common.Target{Target: &common.Target_Weapon{Weapon: &common.Weapon{
		Type: "AIM_120C",
	}}}

	target, _ := buildTarget(protoTarget)

	if target.Kind != models.ObjectKindWeapon {
		t.Errorf("expected kind %q, got %q", models.ObjectKindWeapon, target.Kind)
	}
	if target.Weapon.Type != "AIM_120C" {
		t.Errorf("expected weapon type AIM_120C, got %q", target.Weapon.Type)
	}
	if target.Unit.Type != "" {
		t.Errorf("a weapon target should have no unit, got %q", target.Unit.Type)
	}
}

// Statics were not even parsed: they fell through to a TODO branch.
func TestBuildTargetStatic(t *testing.T) {
	protoTarget := &common.Target{Target: &common.Target_Static{Static: &common.Static{
		Type:      "Warehouse",
		Coalition: common.Coalition_COALITION_BLUE,
	}}}

	target, coalition := buildTarget(protoTarget)

	if target.Kind != models.ObjectKindStatic {
		t.Errorf("expected kind %q, got %q", models.ObjectKindStatic, target.Kind)
	}
	if target.Unit.Type != "Warehouse" {
		t.Errorf("expected Warehouse, got %q", target.Unit.Type)
	}
	if coalition != models.CoalitionBlue {
		t.Errorf("expected blue, got %q", coalition)
	}
}

func TestBuildTargetScenery(t *testing.T) {
	protoTarget := &common.Target{Target: &common.Target_Scenery{Scenery: &common.Scenery{
		Type: "Bridge",
	}}}

	target, coalition := buildTarget(protoTarget)

	if target.Kind != models.ObjectKindScenery {
		t.Errorf("expected kind %q, got %q", models.ObjectKindScenery, target.Kind)
	}
	if target.Unit.Type != "Bridge" {
		t.Errorf("expected Bridge, got %q", target.Unit.Type)
	}
	// Map objects belong to nobody.
	if coalition != models.CoalitionUnknown {
		t.Errorf("expected unknown, got %q", coalition)
	}
}

func TestBuildTargetNil(t *testing.T) {
	target, coalition := buildTarget(nil)

	if target.Kind != models.ObjectKindUnknown {
		t.Errorf("expected kind %q, got %q", models.ObjectKindUnknown, target.Kind)
	}
	if coalition != models.CoalitionUnknown {
		t.Errorf("expected unknown, got %q", coalition)
	}
}
