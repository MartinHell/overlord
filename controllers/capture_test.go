package controllers

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/MartinHell/overlord/models"
)

// Unit rows are deduplicated by type, so instance identity only survives if it
// is captured onto the event.
func TestBuildInitiatorCapturesIdentityAndPosition(t *testing.T) {
	initiator := &common.Initiator{Initiator: &common.Initiator_Unit{Unit: &common.Unit{
		Type:     "F-16C_50",
		Name:     "Aerial-1-1",
		Callsign: "VIPER11",
		Group:    &common.Group{Name: "Viper Flight"},
		Position: &common.Position{Lat: 42.5, Lon: 41.25, Alt: 6000},
	}}}

	got := buildInitiator(initiator)

	if got.Name != "Aerial-1-1" {
		t.Errorf("expected unit name, got %q", got.Name)
	}
	if got.Callsign != "VIPER11" {
		t.Errorf("expected callsign, got %q", got.Callsign)
	}
	if got.Group != "Viper Flight" {
		t.Errorf("expected group name, got %q", got.Group)
	}
	if got.Position.Lat != 42.5 || got.Position.Lon != 41.25 || got.Position.Alt != 6000 {
		t.Errorf("expected position to be captured, got %+v", got.Position)
	}
	// The type still drives the deduplicated Unit row.
	if got.Unit.Type != "F-16C_50" {
		t.Errorf("expected type F-16C_50, got %q", got.Unit.Type)
	}
}

func TestBuildTargetCapturesPosition(t *testing.T) {
	protoTarget := &common.Target{Target: &common.Target_Unit{Unit: &common.Unit{
		Type:     "Su-27",
		Name:     "Bandit-2",
		Position: &common.Position{Lat: 43.0, Lon: 40.0, Alt: 8000},
	}}}

	_, _, pos := buildTarget(protoTarget)

	if pos.Name != "Bandit-2" {
		t.Errorf("expected target name, got %q", pos.Name)
	}
	if pos.Lat != 43.0 || pos.Alt != 8000 {
		t.Errorf("expected target position, got %+v", pos)
	}
}

func TestPositionOfHandlesNil(t *testing.T) {
	got := positionOf(nil)

	if got.Lat != 0 || got.Lon != 0 || got.Alt != 0 {
		t.Fatalf("expected a zero position for nil, got %+v", got)
	}
}

// Mission boundaries carry nothing but the clock, and must still be stored:
// without them every event ever recorded is one undifferentiated pile.
func TestMissionBoundariesAreNotTreatedAsEmpty(t *testing.T) {
	for _, eventType := range []string{"mission_start", "mission_end"} {
		d := eventDetail{Type: eventType}
		if isEmptyEvent(d, initiatorInfo{}, models.Weapon{}, models.Target{}) {
			t.Errorf("%s must be recorded despite carrying no payload", eventType)
		}
	}
}

// Anything else with nothing identifiable in it is a junk row.
func TestEmptyEventIsSkipped(t *testing.T) {
	d := eventDetail{Type: "hit"}

	if !isEmptyEvent(d, initiatorInfo{}, models.Weapon{}, models.Target{}) {
		t.Fatal("expected an event with no content to be skipped")
	}
}

// A takeoff has no weapon and no target, but the airbase makes it worth keeping.
func TestEventWithOnlyPlaceIsKept(t *testing.T) {
	d := eventDetail{Type: "takeoff", Place: &common.Airbase{Name: "Batumi"}}

	if isEmptyEvent(d, initiatorInfo{}, models.Weapon{}, models.Target{}) {
		t.Fatal("expected an event carrying an airbase to be kept")
	}
}

// A landing grade is the entire value of a landingQualityMark event.
func TestEventWithOnlyCommentIsKept(t *testing.T) {
	d := eventDetail{Type: "landing_quality_mark", Comment: "OK_"}

	if isEmptyEvent(d, initiatorInfo{}, models.Weapon{}, models.Target{}) {
		t.Fatal("expected an event carrying a landing grade to be kept")
	}
}
