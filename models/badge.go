package models

// Badge is one achievement, earned or not.
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
	// Progress out of Target, for the locked rendering. Zero targets mean the
	// badge is all-or-nothing and has no meaningful bar.
	Progress int
	Target   int
	// Detail is the earned story: "24 kills in mission #12".
	Detail string
}
