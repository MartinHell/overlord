package controllers

import (
	"fmt"
	"math"
	"time"

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

// notScenery excludes blast damage to trees, walls and houses. Written against
// a LEFT JOIN, so an event with no target row at all still counts: the target
// being unknown is not the same as it having been a tree.
const notScenery = `(targets.kind IS NULL OR targets.kind <> 'scenery')`

// isCollision recognises DCS naming an airframe as the weapon, which is how it
// reports an aircraft flown into something.
//
// Type equality alone is not enough. A gun hit records the shell as both the
// initiator object and the weapon, so a bare `initiator.type = weapon.type`
// test files 17,624 M61 cannon hits in the current database as rammings. Real
// stores are prefixed `weapons.` by DCS and airframes never are, which
// separates the two cleanly.
//
// Requires the events query to alias units on the initiator as iunits.
const isCollision = `(iunits.type = weapons.type AND weapons.type NOT LIKE 'weapons.%')`

// sceneryUnitIDs selects every units row that has been seen as scenery.
//
// Unit rows carry no kind of their own -- a beech and a Hornet are both just a
// type string -- so the only record of which is which is the kind stored on the
// targets that referenced them. Built fresh per call, since a reused *gorm.DB
// accumulates state.
func sceneryUnitIDs() *gorm.DB {
	return initializers.DB.Model(&models.Target{}).
		Select("targets.unit_id").
		Where("targets.kind = ?", models.ObjectKindScenery)
}

// scopeMission narrows an events-rooted query to one mission when set. Nil
// means all of recorded history, which is what the API defaulted to before
// missions existed, so existing callers keep their meaning.
func scopeMission(missionID *uint) func(*gorm.DB) *gorm.DB {
	return func(q *gorm.DB) *gorm.DB {
		if missionID == nil {
			return q
		}
		return q.Where("events.mission_id = ?", *missionID)
	}
}

// GetMissions lists recorded missions, newest first. Start, size and duration
// are derived from the events rather than stored, so backfilled missions
// report them exactly like live ones.
func GetMissions() ([]*models.MissionSummary, error) {
	// MIN over the id rather than over created_at: an aggregated timestamp
	// comes back from SQLite as a bare string that will not scan into
	// time.Time, where an integer id scans anywhere. The timestamps behind
	// those ids are fetched plainly in a second query.
	var rows []struct {
		MissionID uint
		Name      string
		Theatre   string
		Events    int
		Duration  float64
		FirstID   *uint
	}

	err := initializers.DB.Model(&models.Mission{}).
		Select(`missions.mission_id AS mission_id,
			missions.name AS name,
			missions.theatre AS theatre,
			COUNT(events.id) AS events,
			COALESCE(MAX(events.mission_time), 0) AS duration,
			MIN(events.id) AS first_id`).
		Joins("LEFT JOIN events ON events.mission_id = missions.mission_id").
		Group("missions.mission_id, missions.name, missions.theatre").
		Order("missions.mission_id DESC").
		Scan(&rows).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to list missions: %v", err)
		return nil, err
	}

	var firstIDs []uint
	for _, r := range rows {
		if r.FirstID != nil {
			firstIDs = append(firstIDs, *r.FirstID)
		}
	}

	started := map[uint]time.Time{}
	if len(firstIDs) > 0 {
		var firsts []struct {
			ID        uint
			CreatedAt time.Time
		}
		if err := initializers.DB.Model(&models.Event{}).
			Select("events.id, events.created_at").
			Where("events.id IN ?", firstIDs).
			Scan(&firsts).Error; err != nil {
			logs.Sugar.Errorf("Failed to resolve mission start times: %v", err)
			return nil, err
		}
		for _, f := range firsts {
			started[f.ID] = f.CreatedAt
		}
	}

	result := make([]*models.MissionSummary, 0, len(rows))
	for _, r := range rows {
		summary := &models.MissionSummary{
			MissionID: r.MissionID,
			Name:      r.Name,
			Theatre:   r.Theatre,
			Events:    r.Events,
			Duration:  r.Duration,
		}
		if r.FirstID != nil {
			summary.StartedAt = started[*r.FirstID]
		}
		result = append(result, summary)
	}

	return result, nil
}

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
//
// Scenery is excluded, like every other kill figure. It is the scoreboard on
// the front page, and counting felled trees toward a coalition's total put 5%
// of blue's and 7% of red's score down to woodland.
func GetKillsByCoalition(missionID *uint) ([]*models.CoalitionKills, error) {
	// Rows predating coalition tracking have an empty string rather than
	// "unknown", so normalise before grouping.
	const coalitionExpr = `CASE WHEN events.coalition IS NULL OR events.coalition = '' THEN 'unknown' ELSE events.coalition END`

	var rows []models.CoalitionKills

	err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(coalitionExpr+` AS coalition,
			COUNT(*) AS kills,
			SUM(CASE WHEN events.coalition <> '' AND events.coalition <> 'unknown'
				AND events.target_coalition = events.coalition THEN 1 ELSE 0 END) AS teamkills`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND "+notScenery, "kill").
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
//
// Collisions are counted separately rather than as hits and kills. DCS names
// the airframe as the weapon when an aircraft is flown into something, so an
// A-50 appears as a store with 142 kills to its name. Splitting them out keeps
// the ratios about what the weapon did and gives the client a real number to
// filter on instead of inferring collisions from an absent shot count.
func GetWeaponEffectiveness(missionID *uint) ([]*models.WeaponEffectiveness, error) {
	var rows []models.WeaponEffectiveness

	err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`weapons.type AS weapon_type,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('hit','kill')
				AND `+isCollision+` THEN 1 ELSE 0 END) AS collisions`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units AS iunits ON iunits.unit_id = events.initiator_unit_id").
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
func GetPlayerActivity(missionID *uint) ([]*models.PlayerActivity, error) {
	var rows []models.PlayerActivity

	err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`players.player_id AS player_id,
			players.player_name AS player_name,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('takeoff', 'runway_takeoff') THEN 1 ELSE 0 END) AS takeoffs,
			SUM(CASE WHEN events.event IN ('land', 'runway_touch') THEN 1 ELSE 0 END) AS landings,
			SUM(CASE WHEN events.event = 'crash' THEN 1 ELSE 0 END) AS crashes,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections,
			SUM(CASE WHEN events.event = 'pilot_dead' THEN 1 ELSE 0 END) AS deaths`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
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
func GetLandingGrades(limit int, missionID *uint) ([]*models.LandingGrade, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	var rows []models.LandingGrade

	err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
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
// topFavourite crowns the largest tally in a map. Ties break alphabetically so
// the answer is stable between refreshes; map order alone is not.
func topFavourite(totals map[string]int) *models.Favourite {
	var best *models.Favourite
	for name, n := range totals {
		if n <= 0 {
			continue
		}
		if best == nil || n > best.Count || (n == best.Count && name < best.Name) {
			best = &models.Favourite{Name: name, Count: n}
		}
	}
	return best
}

// Several queries rather than one. They group by different things -- airframe,
// weapon, matchup -- so a single statement would need either a join per
// dimension, multiplying rows, or a window function, which is not portable
// between SQLite and Postgres. Each is a grouped aggregate over an indexed
// player_id, not a scan into Go.
// unitType narrows the whole profile to one airframe when non-empty, which is
// what the per-model page asks for.
func GetPlayerProfile(playerID uint, unitType string, missionID *uint) (*models.PlayerProfileView, error) {
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
		Scopes(scopeMission(missionID)).
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
		Scopes(scopeMission(missionID)).
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
		Scopes(scopeMission(missionID)).
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
		Scopes(scopeMission(missionID)).
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

	// Per weapon. Collisions are split out exactly as in
	// GetWeaponEffectiveness, so a pilot's own airframe cannot be crowned
	// their weapon of choice on the strength of what it rammed.
	var weapons []models.WeaponEffectiveness
	if err := onUnit(db.Model(&models.Event{})).
		Scopes(scopeMission(missionID)).
		Select(`weapons.type AS weapon_type,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit'
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('hit','kill')
				AND `+isCollision+` THEN 1 ELSE 0 END) AS collisions`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units AS iunits ON iunits.unit_id = events.initiator_unit_id").
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
		Scopes(scopeMission(missionID)).
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
	// Built fresh per use rather than shared: the favourites below need the
	// same subquery, and a *gorm.DB accumulates state when reused.
	victimNames := func() *gorm.DB {
		return onUnit(db.Model(&models.Event{})).
			Scopes(scopeMission(missionID)).
			Select("DISTINCT events.initiator_name").
			Where("events.player_id = ? AND events.initiator_name <> ?", playerID, "")
	}

	var killedBy []models.Matchup
	killedByQuery := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("tunits.type AS unit_type, units.type AS target_type, COUNT(*) AS kills").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND events.player_id <> ? AND events.target_name IN (?)",
			"kill", playerID, victimNames())

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

	// Favourites: one standout answer per dossier line. The first four are
	// maxima over aggregates computed above; the rest need their own queries
	// because killedBy grouped away the weapon and the killer, and nothing so
	// far has touched the mission's theatre.
	fav := &models.Favourites{}

	for _, a := range view.Aircraft { // already ordered most sorties first
		if a.Sorties > 0 {
			fav.Aircraft = &models.Favourite{Name: a.UnitType, Count: a.Sorties}
			break
		}
	}

	// Most kills; the list is ordered most fired first, so a strict > hands
	// ties to the weapon they actually reach for.
	for _, w := range view.Weapons {
		if w.Kills > 0 && (fav.Weapon == nil || w.Kills > fav.Weapon.Count) {
			fav.Weapon = &models.Favourite{Name: w.WeaponType, Count: w.Kills}
		}
	}

	prey := map[string]int{}
	for _, m := range view.Matchups {
		prey[m.TargetType] += m.Kills
	}
	fav.Prey = topFavourite(prey)

	// On the killedBy list TargetType is what did the killing; see Matchup.
	nemeses := map[string]int{}
	for _, m := range view.KilledBy {
		nemeses[m.TargetType] += m.Kills
	}
	fav.NemesisUnit = topFavourite(nemeses)

	var nemesisPilot struct {
		ID    uint
		Name  string
		Total int
	}
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("players.player_id AS id, players.player_name AS name, COUNT(*) AS total").
		Joins("JOIN players ON players.player_id = events.player_id").
		Where("events.event = ? AND events.player_id <> ? AND events.target_name IN (?)",
			"kill", playerID, victimNames()).
		Group("players.player_id, players.player_name").
		Order("total DESC, players.player_name").
		Limit(1).
		Scan(&nemesisPilot).Error; err != nil {
		logs.Sugar.Errorf("Failed to find nemesis pilot for player %d: %v", playerID, err)
		return nil, err
	}
	if nemesisPilot.Total > 0 {
		id := nemesisPilot.ID
		fav.NemesisPilot = &models.Favourite{ID: &id, Name: nemesisPilot.Name, Count: nemesisPilot.Total}
	}

	var deadliest struct {
		Name  string
		Total int
	}
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("weapons.type AS name, COUNT(*) AS total").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.event = ? AND events.player_id <> ? AND events.target_name IN (?)",
			"kill", playerID, victimNames()).
		Group("weapons.type").
		Order("total DESC, weapons.type").
		Limit(1).
		Scan(&deadliest).Error; err != nil {
		logs.Sugar.Errorf("Failed to find deadliest weapon for player %d: %v", playerID, err)
		return nil, err
	}
	if deadliest.Total > 0 {
		fav.DeadliestWeapon = &models.Favourite{Name: deadliest.Name, Count: deadliest.Total}
	}

	// Scenery excluded to match every other kill count on the profile.
	var theatre struct {
		Name  string
		Total int
	}
	if err := onUnit(db.Model(&models.Event{})).
		Scopes(scopeMission(missionID)).
		Select("missions.theatre AS name, COUNT(*) AS total").
		Joins("JOIN missions ON missions.mission_id = events.mission_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND events.player_id = ? AND missions.theatre <> ''", "kill", playerID).
		Where("targets.kind IS NULL OR targets.kind <> ?", models.ObjectKindScenery).
		Group("missions.theatre").
		Order("total DESC, missions.theatre").
		Limit(1).
		Scan(&theatre).Error; err != nil {
		logs.Sugar.Errorf("Failed to find favourite theatre for player %d: %v", playerID, err)
		return nil, err
	}
	if theatre.Total > 0 {
		fav.Theatre = &models.Favourite{Name: theatre.Name, Count: theatre.Total}
	}

	view.Favourites = fav

	sorties, err := GetSorties(playerID, missionID, unitType)
	if err != nil {
		return nil, err
	}
	view.SortieLog = sorties

	if view.Rivalries, err = GetRivalries(playerID, missionID); err != nil {
		return nil, err
	}
	if view.Titles, err = GetTitles(playerID); err != nil {
		return nil, err
	}

	// Graded landings, newest first.
	var grades []models.LandingGrade
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
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
		Scopes(scopeMission(missionID)).
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

	// Where the kills happened. The victim's position wins because that is
	// where the thing died; the shooter's position is the fallback.
	//
	// LEFT JOIN on targets, deliberately. Two thirds of kills have no target
	// row at all -- DCS had already deallocated the victim by the time the
	// event fired -- and an inner join silently dropped every one of them. The
	// position is on the event either way, so those kills are perfectly
	// mappable; only the name of what died is missing.
	var points []models.KillPoint
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`CASE WHEN events.target_lat <> 0 THEN events.target_lat ELSE events.initiator_lat END AS lat,
			CASE WHEN events.target_lat <> 0 THEN events.target_lon ELSE events.initiator_lon END AS lon,
			tunits.type AS target_type,
			weapons.type AS weapon_type,
			events.mission_time AS mission_time`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("LEFT JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where(`events.event = 'kill' AND events.player_id = ? AND `+notScenery+`
			AND (events.target_lat <> 0 OR events.initiator_lat <> 0)`, playerID).
		Scopes(whereUnitType(unitType)).
		Order("events.id DESC").
		Limit(1000).
		Scan(&points).Error; err != nil {
		logs.Sugar.Errorf("Failed to load kill points for player %d: %v", playerID, err)
		return nil, err
	}
	for i := range points {
		view.KillPoints = append(view.KillPoints, &points[i])
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

// killRecordRow is a kill with everything needed to describe it.
type killRecordRow struct {
	PlayerID     uint
	PlayerName   string
	UnitType     string
	WeaponType   string
	TargetType   string
	MissionTime  float64
	InitiatorLat float64
	InitiatorLon float64
	InitiatorAlt float64
	TargetLat    float64
	TargetLon    float64
}

// killRecords selects real kills -- something that could shoot back -- with the
// names attached. Scenery is excluded, so "first blood" is never a shrub.
func killRecords(missionID *uint) *gorm.DB {
	return initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`events.player_id AS player_id,
			players.player_name AS player_name,
			units.type AS unit_type,
			weapons.type AS weapon_type,
			tunits.type AS target_type,
			events.mission_time AS mission_time,
			events.initiator_lat AS initiator_lat,
			events.initiator_lon AS initiator_lon,
			events.initiator_alt AS initiator_alt,
			events.target_lat AS target_lat,
			events.target_lon AS target_lon`).
		Joins("LEFT JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Where("events.event = ? AND targets.kind <> ?", "kill", models.ObjectKindScenery)
}

func toKillRecord(r killRecordRow, rangeM float64) *models.KillRecord {
	return &models.KillRecord{
		PlayerID:    r.PlayerID,
		PlayerName:  r.PlayerName,
		UnitType:    r.UnitType,
		WeaponType:  r.WeaponType,
		TargetType:  r.TargetType,
		MissionTime: r.MissionTime,
		RangeM:      rangeM,
		AltitudeM:   r.InitiatorAlt,
	}
}

// GetRecords finds the standout moments: the first kill, the longest, the
// highest, and the weapon that converted best.
func GetRecords(missionID *uint) (*models.Records, error) {
	out := &models.Records{}

	// First blood. mission_time > 0 skips rows recorded before the clock was
	// captured, which would otherwise always win.
	var first killRecordRow
	err := killRecords(missionID).
		Where("events.mission_time > 0").
		Order("events.mission_time ASC").
		Limit(1).
		Scan(&first).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to find first blood: %v", err)
		return nil, err
	}
	if first.MissionTime > 0 {
		out.FirstBlood = toKillRecord(first, 0)
	}

	// Highest kill, by the shooter's altitude.
	var highest killRecordRow
	if err := killRecords(missionID).
		Where("events.initiator_alt > 0").
		Order("events.initiator_alt DESC").
		Limit(1).
		Scan(&highest).Error; err != nil {
		logs.Sugar.Errorf("Failed to find highest kill: %v", err)
		return nil, err
	}
	if highest.InitiatorAlt > 0 {
		out.HighestKill = toKillRecord(highest, 0)
	}

	// Longest kill. Great-circle distance is not expressible portably in SQL,
	// so the candidates come back and the maximum is found here. Only kills
	// carrying both positions qualify, which is roughly half of them.
	var candidates []killRecordRow
	if err := killRecords(missionID).
		Where("events.initiator_lat <> 0 AND events.target_lat <> 0").
		Scan(&candidates).Error; err != nil {
		logs.Sugar.Errorf("Failed to load kill geometry: %v", err)
		return nil, err
	}

	best := 0.0
	for _, c := range candidates {
		d := haversineM(c.InitiatorLat, c.InitiatorLon, c.TargetLat, c.TargetLon)
		if d > best {
			best = d
			out.LongestKill = toKillRecord(c, d)
		}
	}

	// Most efficient weapon, over a sample big enough to mean something. One
	// lucky shot would otherwise take this every time.
	weapons, err := GetWeaponEffectiveness(missionID)
	if err != nil {
		return nil, err
	}
	const minShots = 10
	for _, w := range weapons {
		if w.Shots < minShots || w.Kills == 0 {
			continue
		}
		if out.Deadliest == nil || w.KillsPerShot() > out.Deadliest.KillsPerShot {
			out.Deadliest = &models.WeaponRecord{
				WeaponType:   w.WeaponType,
				Shots:        w.Shots,
				Kills:        w.Kills,
				KillsPerShot: w.KillsPerShot(),
			}
		}
	}

	return out, nil
}

// haversineM is great-circle distance in metres. DCS positions are degrees on
// a spherical earth, and at engagement ranges the difference between this and
// a proper ellipsoid is centimetres.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000.0

	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)

	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// GetKillPoints returns every geolocated kill in scope, both sides, for the
// mission map. The victim's position wins; the shooter's is the fallback.
//
// The targets join is a LEFT JOIN because most kills have no target row. DCS
// fires the kill event after it has already deallocated the victim, so about
// two thirds of them name nothing -- but every kill carries coordinates, so
// they belong on the map regardless. An inner join here was hiding 888 of
// roughly 1,400 kills, which is why the map used to look so sparse.
func GetKillPoints(missionID *uint) ([]*models.MapKillPoint, error) {
	var points []models.MapKillPoint

	err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`CASE WHEN events.target_lat <> 0 THEN events.target_lat ELSE events.initiator_lat END AS lat,
			CASE WHEN events.target_lat <> 0 THEN events.target_lon ELSE events.initiator_lon END AS lon,
			events.coalition AS coalition,
			players.player_name AS player_name,
			units.type AS unit_type,
			tunits.type AS target_type,
			weapons.type AS weapon_type,
			events.mission_time AS mission_time`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("LEFT JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where(`events.event = 'kill' AND `+notScenery+`
			AND (events.target_lat <> 0 OR events.initiator_lat <> 0)`).
		Order("events.id DESC").
		Limit(2000).
		Scan(&points).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to load mission kill points: %v", err)
		return nil, err
	}

	result := make([]*models.MapKillPoint, 0, len(points))
	for i := range points {
		result = append(result, &points[i])
	}

	return result, nil
}

// GetCollateral counts what was hit that was never a threat: trees, walls,
// houses, lamp posts. Pass a player to get only theirs, or nil for the mission.
//
// This is the one place scenery is counted rather than filtered out. Everywhere
// else it is excluded, because a bomb catching a wood is not marksmanship and
// counting it would flatter every weapon with a warhead.
func GetCollateral(playerID *uint, missionID *uint) (*models.Collateral, error) {
	var rows []struct {
		Type   string
		Struck int
		Killed int
	}

	q := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`units.type AS type,
			SUM(CASE WHEN events.event = 'hit' THEN 1 ELSE 0 END) AS struck,
			SUM(CASE WHEN events.event = 'kill' THEN 1 ELSE 0 END) AS killed`).
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units ON units.unit_id = targets.unit_id").
		Where("targets.kind = ? AND events.event IN ?", models.ObjectKindScenery, []string{"hit", "kill"}).
		Group("units.type").
		Order("struck DESC, units.type")

	if playerID != nil {
		q = q.Where("events.player_id = ?", *playerID)
	}

	if err := q.Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate collateral damage: %v", err)
		return nil, err
	}

	out := &models.Collateral{}
	for _, r := range rows {
		out.Struck += r.Struck
		out.Levelled += r.Killed

		tree := models.IsTree(r.Type)
		if tree {
			out.Trees += r.Struck
		} else {
			out.Structures += r.Struck
		}

		// The tail is long and mostly ones, so only the leaderboard travels.
		if len(out.Top) < 10 {
			out.Top = append(out.Top, &models.SceneryCount{Type: r.Type, Count: r.Struck, Tree: tree})
		}
	}

	return out, nil
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

	// Scenery is excluded from hits and kills here as it is everywhere else.
	// Without the guard an airframe's card credited it with every tree its
	// bombs caught, which for a strike aircraft is most of its record.
	err := initializers.DB.Model(&models.Event{}).
		Select(`SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit' AND `+notScenery+` THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill' AND `+notScenery+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('crash','dead','unit_lost','pilot_dead') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
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
		Where("events.event = ? AND units.type = ? AND targets.kind <> ?",
			"kill", unitType, models.ObjectKindScenery).
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
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill'
				AND `+notScenery+` AND NOT `+isCollision+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('hit','kill')
				AND `+isCollision+` THEN 1 ELSE 0 END) AS collisions`).
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN units AS iunits ON iunits.unit_id = events.initiator_unit_id").
		Where("weapons.type = ?", weaponType).
		Scan(&totals).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to aggregate weapon profile for %q: %v", weaponType, err)
		return nil, err
	}

	view.Shots = totals.Shots
	view.Hits = totals.Hits
	view.Kills = totals.Kills
	view.Collisions = totals.Collisions
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
