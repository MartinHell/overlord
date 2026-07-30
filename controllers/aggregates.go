package controllers

import (
	"fmt"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"gorm.io/gorm"
)

// Aggregations run as GROUP BY in the database rather than by loading every
// matching event into memory and counting in Go. These are the queries a
// dashboard calls on every page load, so they are the ones that must not scale
// with the size of the events table.
//
// Two portability constraints, since overlord runs on both SQLite and Postgres:
// Postgres requires every non-aggregated selected column to appear in the GROUP
// BY, and COUNT(*) FILTER is not portable, so conditional counts use
// SUM(CASE WHEN ...).

// shotRow is one row of the grouped result, before it is nested for GraphQL.
type shotRow struct {
	PlayerID   uint
	PlayerName string
	UnitType   string
	WeaponType string
	Total      int
}

// shotQuery groups shot events by player, initiating unit type and weapon type.
// Joins are inner: a shot with no unit or no weapon cannot contribute to a
// per-unit or per-weapon breakdown.
func shotQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.Event{}).
		Select(`players.player_id AS player_id,
			players.player_name AS player_name,
			units.type AS unit_type,
			weapons.type AS weapon_type,
			COUNT(*) AS total`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.event = ?", "shot").
		Group("players.player_id, players.player_name, units.type, weapons.type")
}

// GetShotsBreakdown totals shots by initiating unit type and weapon type.
func GetShotsBreakdown() ([]*models.UnitWeaponBreakdown, error) {
	var rows []shotRow

	err := initializers.DB.Model(&models.Event{}).
		Select("units.type AS unit_type, weapons.type AS weapon_type, COUNT(*) AS total").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.event = ?", "shot").
		Group("units.type, weapons.type").
		Order("units.type, weapons.type").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate shots by unit: %v", err)
		return nil, err
	}

	var result []*models.UnitWeaponBreakdown
	byUnit := map[string]*models.UnitWeaponBreakdown{}

	for _, row := range rows {
		unit := byUnit[row.UnitType]
		if unit == nil {
			unit = &models.UnitWeaponBreakdown{Unit: row.UnitType}
			byUnit[row.UnitType] = unit
			result = append(result, unit)
		}
		unit.Weapons = append(unit.Weapons, &models.WeaponShotBreakdown{
			WeaponType: row.WeaponType,
			Count:      row.Total,
		})
	}

	return result, nil
}

// GetShotsByPlayers totals shots per player, unit type and weapon type.
func GetShotsByPlayers() ([]*models.PlayerShotBreakdown, error) {
	var rows []shotRow

	err := shotQuery(initializers.DB).
		Order("players.player_name, units.type, weapons.type").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate shots by player: %v", err)
		return nil, err
	}

	return nestShotRows(rows), nil
}

// GetShotsByPlayer totals shots for a single player.
func GetShotsByPlayer(playerID uint) (*models.PlayerShotBreakdown, error) {
	var rows []shotRow

	err := shotQuery(initializers.DB).
		Where("events.player_id = ?", playerID).
		Order("units.type, weapons.type").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate shots for player %d: %v", playerID, err)
		return nil, err
	}

	nested := nestShotRows(rows)
	if len(nested) == 0 {
		return nil, nil
	}

	return nested[0], nil
}

// nestShotRows turns the flat grouped result into the nested shape GraphQL
// expects. This runs over the grouped rows, not over every event.
func nestShotRows(rows []shotRow) []*models.PlayerShotBreakdown {
	var result []*models.PlayerShotBreakdown

	byPlayer := map[uint]*models.PlayerShotBreakdown{}
	byUnit := map[uint]map[string]*models.UnitShotBreakdown{}

	for _, row := range rows {
		player := byPlayer[row.PlayerID]
		if player == nil {
			player = &models.PlayerShotBreakdown{
				PlayerID:   row.PlayerID,
				PlayerName: row.PlayerName,
			}
			byPlayer[row.PlayerID] = player
			byUnit[row.PlayerID] = map[string]*models.UnitShotBreakdown{}
			result = append(result, player)
		}

		unit := byUnit[row.PlayerID][row.UnitType]
		if unit == nil {
			unit = &models.UnitShotBreakdown{UnitType: row.UnitType}
			byUnit[row.PlayerID][row.UnitType] = unit
			player.Units = append(player.Units, unit)
		}

		unit.Weapons = append(unit.Weapons, &models.WeaponShotBreakdown{
			WeaponType: row.WeaponType,
			Count:      row.Total,
		})
	}

	return result
}

// GetKillsByCoalition tallies kills and teamkills per initiating coalition.
//
// A teamkill only counts when both sides are known: two unknown coalitions
// compare equal but say nothing about whose side anyone was on, and every event
// recorded before coalition tracking existed is unknown.
func GetKillsByCoalition() ([]*models.CoalitionKills, error) {
	// Rows predating coalition tracking have an empty string rather than
	// "unknown", so normalise before grouping.
	const coalitionExpr = `CASE WHEN events.coalition IS NULL OR events.coalition = '' THEN 'unknown' ELSE events.coalition END`

	var rows []models.CoalitionKills

	err := initializers.DB.Model(&models.Event{}).
		Select(coalitionExpr+` AS coalition,
			COUNT(*) AS kills,
			SUM(CASE WHEN events.coalition <> '' AND events.coalition <> 'unknown'
				AND events.target_coalition = events.coalition THEN 1 ELSE 0 END) AS teamkills`).
		Where("events.event = ?", "kill").
		Group(coalitionExpr).
		Order(coalitionExpr).
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate kills by coalition: %v", err)
		return nil, err
	}

	result := make([]*models.CoalitionKills, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}

// GetWeaponEffectiveness totals shots, hits and kills per weapon type.
//
// Hits and kills against scenery are excluded: DCS reports a blast catching a
// tree as a hit, and counting those would inflate accuracy for anything with a
// warhead. Shots have no target, so they are counted unconditionally.
func GetWeaponEffectiveness() ([]*models.WeaponEffectiveness, error) {
	var rows []models.WeaponEffectiveness

	err := initializers.DB.Model(&models.Event{}).
		Select(`weapons.type AS weapon_type,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event IN ?", []string{"shot", "hit", "kill"}).
		Group("weapons.type").
		Order("weapons.type").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate weapon effectiveness: %v", err)
		return nil, err
	}

	result := make([]*models.WeaponEffectiveness, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}

// GetPlayerActivity summarises sorties per player: how many started, how many
// ended in a landing, and how many ended badly.
func GetPlayerActivity() ([]*models.PlayerActivity, error) {
	var rows []models.PlayerActivity

	err := initializers.DB.Model(&models.Event{}).
		Select(`players.player_id AS player_id,
			players.player_name AS player_name,
			SUM(CASE WHEN events.event IN ('takeoff', 'runway_takeoff') THEN 1 ELSE 0 END) AS takeoffs,
			SUM(CASE WHEN events.event IN ('land', 'runway_touch') THEN 1 ELSE 0 END) AS landings,
			SUM(CASE WHEN events.event = 'crash' THEN 1 ELSE 0 END) AS crashes,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections,
			SUM(CASE WHEN events.event = 'pilot_dead' THEN 1 ELSE 0 END) AS deaths`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Group("players.player_id, players.player_name").
		Order("players.player_name").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate player activity: %v", err)
		return nil, err
	}

	result := make([]*models.PlayerActivity, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}

// GetLandingGrades returns recent graded landings, newest first. The grade is
// free text as DCS reported it, which for carrier traps is the wire and the
// deviations.
func GetLandingGrades(limit int) ([]*models.LandingGrade, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	var rows []models.LandingGrade

	err := initializers.DB.Model(&models.Event{}).
		Select(`players.player_name AS player_name,
			units.type AS unit_type,
			events.place AS place,
			events.comment AS grade,
			events.mission_time AS mission_time`).
		Joins("LEFT JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ?", "landing_quality_mark").
		Order("events.id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to query landing grades: %v", err)
		return nil, err
	}

	result := make([]*models.LandingGrade, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}

// GetPlayerProfile assembles everything recorded about one player.
//
// Works for the synthetic AI players as well as humans, since those are
// ordinary player rows: asking for the blue AI gives that whole side's record
// in the same shape as a person's.
//
// Several queries rather than one. They group by different things -- airframe,
// weapon, matchup -- so a single statement would need either a join per
// dimension, multiplying rows, or a window function, which is not portable
// between SQLite and Postgres. Each is a grouped aggregate over an indexed
// player_id, not a scan into Go.
// unitType narrows the whole profile to one airframe when non-empty, which is
// what the per-model page asks for.
func GetPlayerProfile(playerID uint, unitType string) (*models.PlayerProfileView, error) {
	var player models.Player
	if err := player.GetPlayerByPlayerID(playerID); err != nil {
		logs.Sugar.Errorf("Failed to load player %d: %v", playerID, err)
		return nil, nil
	}

	name := player.GetPlayerName()
	view := &models.PlayerProfileView{
		PlayerID:   playerID,
		PlayerName: name,
		IsAI:       models.IsAIPlayerName(name),
		UnitType:   unitType,
	}

	db := initializers.DB

	// Narrowing to an airframe means joining the initiator's unit and
	// constraining it. Queries that already join units add the constraint
	// instead, so this is only for the ones that do not.
	onUnit := func(q *gorm.DB) *gorm.DB {
		if unitType == "" {
			return q
		}
		return q.Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
			Where("units.type = ?", unitType)
	}

	// Everything flown, whatever the current narrowing -- this drives the
	// filter, so it must not filter itself out of existence.
	//
	// Ordered by how much the pilot actually used each one, not alphabetically:
	// the client shows the first handful as chips, so the ordering decides
	// which airframes are one click away.
	var flownRows []struct {
		Type  string
		Total int
	}
	if err := db.Model(&models.Event{}).
		Select("units.type AS type, COUNT(*) AS total").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.player_id = ? AND events.initiator_kind = ?", playerID, models.ObjectKindUnit).
		Group("units.type").
		Order("total DESC, units.type").
		Scan(&flownRows).Error; err != nil {
		logs.Sugar.Errorf("Failed to list airframes for player %d: %v", playerID, err)
		return nil, err
	}
	for _, f := range flownRows {
		view.Flown = append(view.Flown, f.Type)
	}

	// Totals. Hits and kills exclude scenery for the same reason as
	// GetWeaponEffectiveness: DCS reports a blast catching a tree as a hit, and
	// counting those would flatter anything with a warhead.
	var totals struct {
		Sorties, Landings, Crashes, Ejections, Deaths int
		Shots, Hits, Kills, Teamkills                 int
		FirstSeen, LastSeen                           float64
	}
	err := onUnit(db.Model(&models.Event{})).
		Select(`SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event IN ('land','runway_touch') THEN 1 ELSE 0 END) AS landings,
			SUM(CASE WHEN events.event = 'crash' THEN 1 ELSE 0 END) AS crashes,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections,
			SUM(CASE WHEN events.event = 'pilot_dead' THEN 1 ELSE 0 END) AS deaths,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event = 'kill' AND events.coalition <> '' AND events.coalition <> 'unknown'
				AND events.target_coalition = events.coalition THEN 1 ELSE 0 END) AS teamkills,
			MIN(events.mission_time) AS first_seen,
			MAX(events.mission_time) AS last_seen`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ?", playerID).
		Scan(&totals).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate totals for player %d: %v", playerID, err)
		return nil, err
	}

	view.Sorties, view.Landings = totals.Sorties, totals.Landings
	view.Crashes, view.Ejections, view.Deaths = totals.Crashes, totals.Ejections, totals.Deaths
	view.Shots, view.Hits, view.Kills = totals.Shots, totals.Hits, totals.Kills
	view.Teamkills = totals.Teamkills
	view.FirstSeen, view.LastSeen = totals.FirstSeen, totals.LastSeen

	// Sides flown, busiest first.
	var sides []struct {
		Coalition string
		Total     int
	}
	if err := onUnit(db.Model(&models.Event{})).
		Select("events.coalition AS coalition, COUNT(*) AS total").
		Where("events.player_id = ? AND events.coalition <> '' AND events.coalition <> 'unknown'", playerID).
		Group("events.coalition").
		Order("total DESC").
		Scan(&sides).Error; err != nil {
		logs.Sugar.Errorf("Failed to list coalitions for player %d: %v", playerID, err)
		return nil, err
	}
	for _, s := range sides {
		view.Coalitions = append(view.Coalitions, s.Coalition)
	}

	// Per airframe.
	var aircraft []models.PlayerAircraftStats
	if err := db.Model(&models.Event{}).
		Select(`units.type AS unit_type,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event IN ('land','runway_touch') THEN 1 ELSE 0 END) AS landings,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('crash','dead','unit_lost','pilot_dead') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ? AND events.initiator_kind = ?", playerID, models.ObjectKindUnit).
		Scopes(whereUnitType(unitType)).
		Group("units.type").
		Order("sorties DESC, kills DESC, units.type").
		Scan(&aircraft).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate aircraft for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range aircraft {
		view.Aircraft = append(view.Aircraft, &aircraft[i])
	}

	// Per weapon.
	var weapons []models.WeaponEffectiveness
	if err := onUnit(db.Model(&models.Event{})).
		Select(`weapons.type AS weapon_type,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ?", playerID).
		Group("weapons.type").
		Order("shots DESC, kills DESC, weapons.type").
		Scan(&weapons).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate weapons for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range weapons {
		view.Weapons = append(view.Weapons, &weapons[i])
	}

	// Matchups: what this player killed, by the airframe they were flying.
	var matchups []models.Matchup
	if err := db.Model(&models.Event{}).
		Select("units.type AS unit_type, tunits.type AS target_type, COUNT(*) AS kills").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Where("events.event = ? AND events.player_id = ? AND targets.kind <> ?",
			"kill", playerID, models.ObjectKindScenery).
		Scopes(whereUnitType(unitType)).
		Group("units.type, tunits.type").
		Order("kills DESC, units.type, tunits.type").
		Scan(&matchups).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate matchups for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range matchups {
		view.Matchups = append(view.Matchups, &matchups[i])
	}

	// The reverse: what killed this player.
	//
	// Target rows are deduplicated by type and carry no player, so there is no
	// foreign key to follow back. What does identify the victim is the DCS unit
	// name on the kill event, which is the same name that appears as the
	// initiator on that player's own events. Matching on it is therefore exact
	// within a mission; across missions a reused slot name could in principle
	// be credited to the wrong player.
	playerUnits := onUnit(db.Model(&models.Event{})).
		Select("DISTINCT events.initiator_name").
		Where("events.player_id = ? AND events.initiator_name <> ?", playerID, "")

	var killedBy []models.Matchup
	killedByQuery := db.Model(&models.Event{}).
		Select("tunits.type AS unit_type, units.type AS target_type, COUNT(*) AS kills").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND events.player_id <> ? AND events.target_name IN (?)",
			"kill", playerID, playerUnits)

	// Here the player's airframe is the victim, so the narrowing applies to the
	// target side rather than the initiator.
	if unitType != "" {
		killedByQuery = killedByQuery.Where("tunits.type = ?", unitType)
	}

	if err := killedByQuery.
		Group("tunits.type, units.type").
		Order("kills DESC, tunits.type, units.type").
		Scan(&killedBy).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate deaths for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range killedBy {
		view.KilledBy = append(view.KilledBy, &killedBy[i])
		view.TimesKilled += killedBy[i].Kills
	}

	// Graded landings, newest first.
	var grades []models.LandingGrade
	if err := db.Model(&models.Event{}).
		Select(`players.player_name AS player_name,
			units.type AS unit_type,
			events.place AS place,
			events.comment AS grade,
			events.mission_time AS mission_time`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND events.player_id = ?", "landing_quality_mark", playerID).
		Scopes(whereUnitType(unitType)).
		Order("events.id DESC").
		Limit(defaultPageSize).
		Scan(&grades).Error; err != nil {
		logs.Sugar.Errorf("Failed to query landing grades for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range grades {
		view.LandingGrades = append(view.LandingGrades, &grades[i])
	}

	// Activity over the mission clock. Bucketed in the database rather than by
	// returning every event and binning them in the browser.
	//
	// The bucket is sized from the span so the shape stays readable whether the
	// mission ran twenty minutes or six hours, and is reported alongside the
	// data so the client does not have to guess what it is drawing.
	bucket := 60
	if span := totals.LastSeen - totals.FirstSeen; span > 0 {
		for span/float64(bucket) > 60 {
			bucket *= 2
		}
	}
	view.BucketSeconds = bucket

	var buckets []struct {
		Bucket                        int
		Sorties, Kills, Losses, Shots int
	}
	// bucket is an int computed just above, never user input, so interpolating
	// it into the expression cannot carry anything through.
	bucketExpr := fmt.Sprintf("CAST(events.mission_time / %d AS INTEGER)", bucket)
	// Scenery is excluded here exactly as it is in the totals. Counted plainly,
	// the chart summed 270 kills against a headline of 251 on the same page,
	// which reads as one of the two being wrong.
	if err := onUnit(db.Model(&models.Event{})).
		Select(bucketExpr+` AS bucket,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('crash','dead','unit_lost','pilot_dead') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ? AND events.mission_time > 0", playerID).
		Group(bucketExpr).
		Order(bucketExpr).
		Scan(&buckets).Error; err != nil {
		logs.Sugar.Errorf("Failed to build timeline for player %d: %v", playerID, err)
		return nil, err
	}

	for _, b := range buckets {
		view.Timeline = append(view.Timeline, &models.TimelineBucket{
			T:       float64(b.Bucket * bucket),
			Sorties: b.Sorties,
			Kills:   b.Kills,
			Losses:  b.Losses,
			Shots:   b.Shots,
		})
	}

	return view, nil
}

// whereUnitType constrains a query that already joins units, so callers do not
// have to repeat the empty check.
func whereUnitType(unitType string) func(*gorm.DB) *gorm.DB {
	return func(q *gorm.DB) *gorm.DB {
		if unitType == "" {
			return q
		}
		return q.Where("units.type = ?", unitType)
	}
}

// GetUnitProfile assembles the reference card for one unit type: curated
// identity plus everything the events table knows about it.
func GetUnitProfile(unitType string) (*models.UnitProfileView, error) {
	if unitType == "" {
		return nil, nil
	}

	profile, curated := models.UnitProfile(unitType)

	view := &models.UnitProfileView{
		Type: unitType, Curated: curated,
		Name: profile.Name, Nickname: profile.Nickname, Role: profile.Role,
		Origin: profile.Origin, Maker: profile.Maker, Blurb: profile.Blurb,
		Source: models.UnitSource(unitType),
	}

	if specs, ok := models.UnitSpecs(unitType); ok {
		view.Specs = &specs
	}

	// One pass over the events for this airframe, counted by kind.
	var totals struct {
		Sorties, Shots, Hits, Kills, Losses, Ejections int
	}

	err := initializers.DB.Model(&models.Event{}).
		Select(`SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit' THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill' THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('crash','dead','unit_lost','pilot_dead') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("units.type = ?", unitType).
		Scan(&totals).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate unit profile for %q: %v", unitType, err)
		return nil, err
	}

	view.Sorties = totals.Sorties
	view.Shots = totals.Shots
	view.Hits = totals.Hits
	view.Kills = totals.Kills
	view.Losses = totals.Losses
	view.Ejections = totals.Ejections

	// How often it was on the receiving end, which the initiator counts above
	// cannot show.
	var killed int64
	if err := initializers.DB.Model(&models.Event{}).
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units ON units.unit_id = targets.unit_id").
		Where("events.event = ? AND units.type = ?", "kill", unitType).
		Count(&killed).Error; err != nil {
		logs.Sugar.Errorf("Failed to count losses for %q: %v", unitType, err)
		return nil, err
	}
	view.TimesKilled = int(killed)

	// What it shoots.
	var stores []struct {
		WeaponType string
		Total      int
	}
	if err := initializers.DB.Model(&models.Event{}).
		Select("weapons.type AS weapon_type, COUNT(*) AS total").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.event = ? AND units.type = ?", "shot", unitType).
		Group("weapons.type").
		Order("total DESC, weapons.type").
		Scan(&stores).Error; err != nil {
		logs.Sugar.Errorf("Failed to list stores for %q: %v", unitType, err)
		return nil, err
	}

	for _, s := range stores {
		view.Stores = append(view.Stores, &models.WeaponShotBreakdown{WeaponType: s.WeaponType, Count: s.Total})
	}

	return view, nil
}

// GetWeaponProfile assembles the reference card for one store.
func GetWeaponProfile(weaponType string) (*models.WeaponProfileView, error) {
	if weaponType == "" {
		return nil, nil
	}

	profile, curated := models.WeaponProfile(weaponType)

	view := &models.WeaponProfileView{
		Type: weaponType, Curated: curated,
		Name: profile.Name, Nickname: profile.Nickname, Role: profile.Role,
		Origin: profile.Origin, Maker: profile.Maker, Blurb: profile.Blurb,
		Source: models.WeaponSource(weaponType),
	}

	if specs, ok := models.WeaponSpecs(weaponType); ok {
		view.Specs = &specs
	}

	var totals models.WeaponEffectiveness
	err := initializers.DB.Model(&models.Event{}).
		Select(`SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("weapons.type = ?", weaponType).
		Scan(&totals).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate weapon profile for %q: %v", weaponType, err)
		return nil, err
	}

	view.Shots = totals.Shots
	view.Hits = totals.Hits
	view.Kills = totals.Kills
	view.HitsPerShot = totals.HitsPerShot()
	view.KillsPerShot = totals.KillsPerShot()

	// Who carries it.
	var carriers []struct {
		UnitType string
		Total    int
	}
	if err := initializers.DB.Model(&models.Event{}).
		Select("units.type AS unit_type, COUNT(*) AS total").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND weapons.type = ?", "shot", weaponType).
		Group("units.type").
		Order("total DESC, units.type").
		Scan(&carriers).Error; err != nil {
		logs.Sugar.Errorf("Failed to list carriers for %q: %v", weaponType, err)
		return nil, err
	}

	for _, c := range carriers {
		view.Carriers = append(view.Carriers, &models.UnitShotBreakdown{UnitType: c.UnitType})
		view.Carriers[len(view.Carriers)-1].Weapons = []*models.WeaponShotBreakdown{
			{WeaponType: weaponType, Count: c.Total},
		}
	}

	return view, nil
}
