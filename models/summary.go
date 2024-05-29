package models

type UnitWeaponBreakdown struct {
	Unit    string
	Weapons []*WeaponShotBreakdown
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
