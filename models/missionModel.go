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
