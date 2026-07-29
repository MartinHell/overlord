package models

import "github.com/DCS-gRPC/go-bindings/dcs/v0/common"

// Coalition values as stored in the database and exposed over GraphQL. They are
// plain strings rather than an enum so that an unrecognised coalition from a
// future DCS release is recorded rather than silently dropped.
const (
	CoalitionRed     = "red"
	CoalitionBlue    = "blue"
	CoalitionNeutral = "neutral"
	CoalitionUnknown = "unknown"
)

// CoalitionFromProto converts the gRPC coalition enum into its stored form.
// COALITION_ALL is only meaningful as a query filter, so it maps to unknown
// when it turns up on an actual unit.
func CoalitionFromProto(c common.Coalition) string {
	switch c {
	case common.Coalition_COALITION_RED:
		return CoalitionRed
	case common.Coalition_COALITION_BLUE:
		return CoalitionBlue
	case common.Coalition_COALITION_NEUTRAL:
		return CoalitionNeutral
	default:
		return CoalitionUnknown
	}
}

// CoalitionFromUnit reads the coalition off a unit, tolerating a nil unit.
func CoalitionFromUnit(u *common.Unit) string {
	if u == nil {
		return CoalitionUnknown
	}
	return CoalitionFromProto(u.GetCoalition())
}
