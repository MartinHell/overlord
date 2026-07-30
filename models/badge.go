package models

import "time"

// Badge is one achievement, with the history of every time it was earned.
//
// Badges are computed from the events every time they are asked for, rather
// than stored when first noticed. Stored awards drift: a bug that over-counts
// hands out medals that survive the fix, and a backfill that reshapes history
// leaves stale ones behind. Computing them means the shelf always agrees with
// the data, and an unearned badge can honestly show how far along it is.
type Badge struct {
	ID    string
	Name  string
	Emoji string
	// Description says how it is earned, phrased for the locked state.
	Description string
	Earned      bool
	// Count is how many times it has been earned. Repeatable badges (a gun
	// kill, an ace mission) count every earning; career thresholds are 1.
	Count int
	// Progress out of Target, for the locked rendering. Zero targets mean the
	// badge is all-or-nothing and has no meaningful bar.
	Progress int
	Target   int
	// Detail is the short story for the shelf: "24 kills in mission #4".
	Detail string
	// Awards are the individual earnings, newest first, capped -- Count is
	// the full number even when this list is not.
	Awards []*BadgeAward
}

// BadgeAward is one earning of a badge: which mission, where, when, and what
// happened. Mission-granular on purpose -- individual sorties are not modelled
// yet, so this is as fine as the data honestly slices.
type BadgeAward struct {
	MissionID   uint
	MissionName string
	Theatre     string
	When        time.Time
	Detail      string
}
