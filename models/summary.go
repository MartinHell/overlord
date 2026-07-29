package models

type UnitWeaponBreakdown struct {
	Unit    string
	Weapons []*WeaponShotBreakdown
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
