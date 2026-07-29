package models

import (
	"testing"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
)

func TestCoalitionFromProto(t *testing.T) {
	tests := []struct {
		in   common.Coalition
		want string
	}{
		{common.Coalition_COALITION_RED, CoalitionRed},
		{common.Coalition_COALITION_BLUE, CoalitionBlue},
		{common.Coalition_COALITION_NEUTRAL, CoalitionNeutral},
		// COALITION_ALL is a query filter, not something a unit can be.
		{common.Coalition_COALITION_ALL, CoalitionUnknown},
	}

	for _, tt := range tests {
		if got := CoalitionFromProto(tt.in); got != tt.want {
			t.Errorf("CoalitionFromProto(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCoalitionFromUnitHandlesNil(t *testing.T) {
	if got := CoalitionFromUnit(nil); got != CoalitionUnknown {
		t.Fatalf("expected %q for a nil unit, got %q", CoalitionUnknown, got)
	}

	unit := &common.Unit{Coalition: common.Coalition_COALITION_BLUE}
	if got := CoalitionFromUnit(unit); got != CoalitionBlue {
		t.Fatalf("expected %q, got %q", CoalitionBlue, got)
	}
}
