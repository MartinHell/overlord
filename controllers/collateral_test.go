package controllers

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
)

func sceneryTarget(kind string) *common.Target {
	return &common.Target{Target: &common.Target_Scenery{Scenery: &common.Scenery{Type: kind}}}
}

// A blast catching a tree produces a hit with no initiator and no weapon. There
// is nothing to attribute it to, and these outnumbered real hits four to one.
func TestCollateralHitIsSkipped(t *testing.T) {
	event := &mission.StreamEventsResponse_HitEvent{
		Target: sceneryTarget("EUROPEAN_BEECH"),
	}

	if !isCollateralHit(event) {
		t.Fatal("expected an unattributed scenery hit to be treated as collateral")
	}
}

// A scenery hit that DCS does attribute is still worth keeping: it says someone
// put ordnance somewhere.
func TestAttributedSceneryHitIsKept(t *testing.T) {
	event := &mission.StreamEventsResponse_HitEvent{
		Initiator: &common.Initiator{Initiator: &common.Initiator_Unit{Unit: &common.Unit{Type: "FA-18C_hornet"}}},
		Weapon:    &common.Weapon{Type: "Mk_82"},
		Target:    sceneryTarget("EUROPEAN_BEECH"),
	}

	if isCollateralHit(event) {
		t.Fatal("expected an attributed scenery hit to be kept")
	}
}

// Some events name the weapon without describing it. That still identifies what
// did the damage, so it is not collateral.
func TestHitWithOnlyWeaponNameIsKept(t *testing.T) {
	name := "Su-24M"
	event := &mission.StreamEventsResponse_HitEvent{
		WeaponName: &name,
		Target:     sceneryTarget("HOME1_C"),
	}

	if isCollateralHit(event) {
		t.Fatal("expected a named weapon to keep the hit")
	}
}

func TestHitOnUnitWithInitiatorIsKept(t *testing.T) {
	event := &mission.StreamEventsResponse_HitEvent{
		Initiator: &common.Initiator{Initiator: &common.Initiator_Unit{Unit: &common.Unit{Type: "F-15C"}}},
		Weapon:    &common.Weapon{Type: "AIM_120C"},
		Target:    &common.Target{Target: &common.Target_Unit{Unit: &common.Unit{Type: "Su-27"}}},
	}

	if isCollateralHit(event) {
		t.Fatal("expected a normal air-to-air hit to be kept")
	}
}
