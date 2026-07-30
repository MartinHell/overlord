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
	// Kills excludes scenery, like every other kill figure.
	Kills     int
	Takeoffs  int
	Landings  int
	Crashes   int
	Ejections int
	Deaths    int
}

// PlayerProfileView is everything recorded about one player, human or AI.
//
// The synthetic AI players are first-class here on purpose: red and blue AI are
// separate player rows, so the same page that shows a human's record shows how
// the AI on each side is doing, and the two can be compared directly.
type PlayerProfileView struct {
	PlayerID   uint
	PlayerName string
	// IsAI marks the synthetic per-coalition AI players rather than a human.
	IsAI bool
	// UnitType is the airframe this record is narrowed to, empty for the whole
	// pilot. Flown is every airframe they have flown regardless of narrowing,
	// so the filter still offers a way back out.
	UnitType string
	Flown    []string
	// Coalitions this player has been seen on, busiest first. A human can
	// switch sides between sorties, so this is a list rather than a field.
	Coalitions []string

	Sorties   int
	Landings  int
	Crashes   int
	Ejections int
	Deaths    int
	Shots     int
	Hits      int
	Kills     int
	Teamkills int
	// TimesKilled is how often something of this player's was destroyed by
	// someone else, matched by unit name; see GetPlayerProfile.
	TimesKilled int
	FirstSeen   float64
	LastSeen    float64

	Aircraft      []*PlayerAircraftStats
	Weapons       []*WeaponEffectiveness
	Matchups      []*Matchup
	KilledBy      []*Matchup
	LandingGrades []*LandingGrade

	// BucketSeconds is how wide each Timeline bucket is, sized from the mission
	// span so the shape reads the same for a short mission and a long one.
	BucketSeconds int
	Timeline      []*TimelineBucket

	// KillPoints are where this player's kills happened, for the map. The
	// victim's position where DCS reported one, the shooter's otherwise, and
	// absent entirely when it reported neither -- which is roughly half of
	// kills, a caveat the map has to state rather than hide.
	KillPoints []*KillPoint

	Favourites *Favourites
}

// Favourite is one superlative: a name, how often it earned the title, and a
// player id when the name is a pilot the client can link to.
type Favourite struct {
	ID    *uint
	Name  string
	Count int
}

// Favourites are the dossier lines on a pilot page -- one standout answer per
// question, each a maximum over an aggregate the profile already carries or a
// dedicated query. Every field is nil when there is nothing to crown, which the
// client renders by leaving that line out rather than inventing a zero.
type Favourites struct {
	// Aircraft is the most-flown airframe, by sorties.
	Aircraft *Favourite
	// Weapon is the weapon with the most kills. Nil until something dies to it;
	// the client can fall back to most-fired from the weapons table.
	Weapon *Favourite
	// Prey is the type this player has destroyed most.
	Prey *Favourite
	// NemesisUnit is the type that has shot this player down most.
	NemesisUnit *Favourite
	// NemesisPilot is the pilot credited with the most of this player's deaths.
	// The flattened AI players count like anyone else, so this is often an AI.
	NemesisPilot *Favourite
	// DeadliestWeapon is the weapon this player has died to most.
	DeadliestWeapon *Favourite
	// Theatre is the map this player has the most kills on. Only interesting
	// across missions; under a single-mission scope it is trivially that
	// mission's map, which the client hides.
	Theatre *Favourite
}

// MapKillPoint is one kill with a place and a side, for the mission map that
// shows both coalitions at once.
type MapKillPoint struct {
	Lat         float64
	Lon         float64
	Coalition   string
	PlayerName  string
	UnitType    string
	TargetType  string
	WeaponType  string
	MissionTime float64
}

// KillPoint is one kill with a place.
type KillPoint struct {
	Lat         float64
	Lon         float64
	TargetType  string
	WeaponType  string
	MissionTime float64
}

// TimelineBucket is one slice of the mission clock.
type TimelineBucket struct {
	// T is the mission time at the start of the bucket, in seconds.
	T       float64
	Sorties int
	Kills   int
	Losses  int
	Shots   int
}

// KillDeathRatio counts a death as a pilot death or a crash, which is as close
// to "lost the aircraft" as the event stream gets. Kills with no deaths returns
// the kill count rather than dividing by zero.
func (p *PlayerProfileView) KillDeathRatio() float64 {
	deaths := p.Deaths + p.Crashes
	if deaths == 0 {
		return float64(p.Kills)
	}
	return float64(p.Kills) / float64(deaths)
}

// PlayerAircraftStats is one player's record in one airframe.
type PlayerAircraftStats struct {
	UnitType  string
	Sorties   int
	Landings  int
	Shots     int
	Hits      int
	Kills     int
	Losses    int
	Ejections int
}

// HitsPerShot carries the same caveat as WeaponEffectiveness.HitsPerShot: it is
// a ratio that legitimately exceeds 1, not a percentage.
func (a *PlayerAircraftStats) HitsPerShot() float64 {
	if a.Shots == 0 {
		return 0
	}
	return float64(a.Hits) / float64(a.Shots)
}

func (a *PlayerAircraftStats) KillsPerShot() float64 {
	if a.Shots == 0 {
		return 0
	}
	return float64(a.Kills) / float64(a.Shots)
}

// Matchup is a kill tally for one airframe against one other type.
//
// Read it in the direction the field names imply: UnitType did the killing,
// TargetType was killed. On the KilledBy list the roles are reversed relative to
// the player -- UnitType is what the player was flying when they died and
// TargetType is what killed them -- which keeps a single shape for both tables.
type Matchup struct {
	UnitType   string
	TargetType string
	Kills      int
}

// Records are the standout moments of the mission.
//
// Everything else here is an aggregate -- totals, averages, ratios -- which
// says how a mission went but never what happened in it. These are single
// events, named and timed, because that is what someone actually wants to
// point at afterwards.
type Records struct {
	FirstBlood  *KillRecord
	LongestKill *KillRecord
	HighestKill *KillRecord
	Deadliest   *WeaponRecord
}

// KillRecord is one kill, with enough around it to tell the story.
type KillRecord struct {
	PlayerID    uint
	PlayerName  string
	UnitType    string
	WeaponType  string
	TargetType  string
	MissionTime float64
	// RangeM is how far apart shooter and target were when the target died.
	//
	// Not the launch range, which is not recoverable: DCS gives a position on
	// the shot event but no target, and by the time the kill lands the shooter
	// has flown on. A Phoenix trucking along for a minute makes those two
	// numbers very different, so this is labelled for what it is.
	RangeM float64
	// AltitudeM is the shooter's altitude at the moment of the kill.
	AltitudeM float64
}

// WeaponRecord is the most efficient weapon, over a minimum sample.
type WeaponRecord struct {
	WeaponType   string
	Shots        int
	Kills        int
	KillsPerShot float64
}

// Collateral is what got caught in the blast: map scenery, counted separately
// from anything that shoots back.
//
// Struck and Levelled are different events and the difference matters. DCS
// emits a hit when a blast touches a scenery object and a kill only when it is
// actually destroyed, and on the test mission that was 25,301 against 78. A
// figure labelled "destroyed" that is really the first number would be wrong by
// three hundred times.
type Collateral struct {
	Struck   int
	Levelled int
	// Trees and Structures split Struck. The grouping is read off the DCS
	// identifier, so it is a guess; see IsTree.
	Trees      int
	Structures int
	Top        []*SceneryCount
}

// SceneryCount is one kind of scenery and how often it was hit.
type SceneryCount struct {
	Type  string
	Count int
	Tree  bool
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

// UnitProfileView is a reference card for one airframe or vehicle type, with
// everything overlord has actually recorded about it.
type UnitProfileView struct {
	Type string
	// Curated reports whether the reference data is from the table or was
	// derived mechanically from the DCS identifier.
	Curated  bool
	Name     string
	Nickname string
	Role     string
	Origin   string
	Maker    string
	Blurb    string
	// Source is a canonical article to read further, empty when none is recorded.
	Source string
	// Specs are the generated Wikidata facts. Fields are zero where Wikidata
	// holds nothing, which is common.
	Specs *Specs

	// Recorded, not reference: everything below comes from the events table.
	Sorties     int
	Shots       int
	Hits        int
	Kills       int
	Losses      int
	Ejections   int
	TimesKilled int
	Stores      []*WeaponShotBreakdown
}

// WeaponProfileView is the same idea for a store.
type WeaponProfileView struct {
	Type     string
	Curated  bool
	Name     string
	Nickname string
	Role     string
	Origin   string
	Maker    string
	Blurb    string
	Source   string
	Specs    *Specs

	Shots        int
	Hits         int
	Kills        int
	HitsPerShot  float64
	KillsPerShot float64
	// Carriers are the airframes seen firing it, busiest first.
	Carriers []*UnitShotBreakdown
}
