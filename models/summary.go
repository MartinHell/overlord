package models

type UnitWeaponBreakdown struct {
	Unit    string
	Weapons []*WeaponShotBreakdown
}

// WeaponEffectiveness is shots against hits against kills for one weapon type.
// Recording hits is what makes this possible at all: without them there is no
// way to tell eight missiles fired for two kills from two fired for two.
type WeaponEffectiveness struct {
	WeaponType string
	Shots      int
	Hits       int
	Kills      int
}

// HitsPerShot is hits divided by shots. Deliberately a ratio rather than a
// percentage, because it legitimately exceeds 1: DCS emits one shot event per
// launch but one hit event per object damaged, so a submunition dispenser or
// anything with splash damage registers many hits from a single shot. Measured
// values reached 1.44 for AGM-154.
//
// It is therefore a measure of how much a weapon damages, not of how often it
// finds its target. Guns are the opposite case: DCS emits hits but no shot
// events for cannon fire, so their shot count is zero and this is meaningless.
func (w *WeaponEffectiveness) HitsPerShot() float64 {
	if w.Shots == 0 {
		return 0
	}
	return float64(w.Hits) / float64(w.Shots)
}

// KillsPerShot is kills divided by shots. Also a ratio that can exceed 1, for
// the same reason: one weapon can destroy several units.
func (w *WeaponEffectiveness) KillsPerShot() float64 {
	if w.Shots == 0 {
		return 0
	}
	return float64(w.Kills) / float64(w.Shots)
}

// PlayerActivity summarises a player's sorties and how they ended.
type PlayerActivity struct {
	PlayerID   uint
	PlayerName string
	Takeoffs   int
	Landings   int
	Crashes    int
	Ejections  int
	Deaths     int
}

// LandingGrade is one graded landing, as DCS reported it.
type LandingGrade struct {
	PlayerName  string
	UnitType    string
	Place       string
	Grade       string
	MissionTime float64
}

// CoalitionKills is a kill tally for one coalition.
type CoalitionKills struct {
	Coalition string
	Kills     int
	// Teamkills counts kills where the target was on the same side as the
	// initiator.
	Teamkills int
}

type PlayerShotBreakdown struct {
	PlayerID   uint
	PlayerName string
	Units      []*UnitShotBreakdown
}

type UnitShotBreakdown struct {
	UnitType string
	Weapons  []*WeaponShotBreakdown
}

type WeaponShotBreakdown struct {
	WeaponType string
	Count      int
}
