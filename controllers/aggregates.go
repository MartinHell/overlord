package controllers

import (
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
