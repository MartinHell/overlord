package models

import "time"

// Mission is one run of a DCS mission: from a mission start (or the first
// event after the clock reset) until the next.
//
// This table exists because every aggregate was silently merging all of
// history: fourteen runs deep, "first blood" was frozen at a moment from days
// ago and the activity timeline overlaid fourteen different sessions on one
// clock axis. Events carry a mission id so any query can be scoped to one run.
//
// The row itself is deliberately bare. A mission's start time, duration and
// event count are all derivable from its events, and deriving them means a
// backfilled mission from before this table existed reports them just as well
// as a live one.
type Mission struct {
	MissionID uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	// Name and Theatre are fetched best-effort from DCS when the mission
	// opens. Empty for missions recorded before this existed, and for any
	// where the fetch failed -- absent means unknown, and the UI says so
	// rather than inventing one.
	Name    string
	Theatre string
}

// MissionSummary is a mission with its derived facts, for listing.
type MissionSummary struct {
	MissionID uint
	Name      string
	Theatre   string
	StartedAt time.Time
	Events    int
	// Duration is the highest mission clock seen, in seconds.
	Duration float64
}

// MissionEntry is one row of the mission index: the summary plus what makes
// this run worth opening rather than the one above it.
//
// Kept apart from MissionSummary because the extra work is real -- three more
// queries and a pass over every kill -- and the summary is fetched on every
// page load by the badge shelf, the flight log and the hero. Only the index
// asks for this.
type MissionEntry struct {
	MissionSummary
	// Kills excludes scenery, like every other kill figure.
	Kills int
	// Sorties is takeoffs by anyone, AI included: it is a measure of how busy
	// the sky was.
	Sorties int
	// Pilots are the humans who flew it, busiest first. AI is deliberately
	// absent -- it is in every mission, so it distinguishes none of them.
	Pilots []*MissionPilot
	// Highlight is the one thing worth saying about this run. Nil when nothing
	// stood out, which is honest: some nights are quiet.
	Highlight *MissionHighlight
}

// MissionPilot is one human's presence in a mission.
type MissionPilot struct {
	PlayerID   uint
	PlayerName string
	Kills      int
}

// Highlight kinds, in the order they are preferred. A burst of kills is a
// better story than a total, and a total is better than nothing.
const (
	HighlightMultiKill = "multikill"
	HighlightAce       = "ace"
	HighlightLongShot  = "longshot"
	HighlightTopScorer = "topscorer"
)

// MissionHighlight is the standout moment of one run, phrased by the client
// from these parts rather than here -- wording lives with the rest of the
// wording.
type MissionHighlight struct {
	Kind       string
	PlayerID   *uint
	PlayerName string
	// Count is kills: in the burst, in the mission, or the scorer's total.
	Count int
	// Seconds is how long a multikill took; Nm the range of a long shot.
	// Each is zero when the kind does not use it.
	Seconds float64
	Nm      float64
}
