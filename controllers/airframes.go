package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// Airframes: every type that has flown, side by side.
//
// The reference card already answers "how did the Hornet do"; this answers
// "how did the Hornet do compared with everything else", which is the question
// the weapons table has been answering for stores all along.
//
// Restricted to types seen as an initiator of something, which is what keeps
// the scenery out without a second rule: a tree never took off or fired.

// GetAirframes totals one row per aircraft or vehicle type.
func GetAirframes(missionID *uint) ([]*models.AirframeStats, error) {
	var rows []models.AirframeStats

	// Collisions are held apart from kills, as they are on the weapons page and
	// for the same reason. DCS names the airframe as the weapon when an
	// aircraft goes into something, so counting those as kills made the A-50 --
	// an airborne radar with no weapons at all -- the third deadliest aircraft
	// on the server with 135 of them.
	//
	// Same test as isCollision, written against this query's aliases.
	const rammed = `(weapons.type = units.type AND weapons.type NOT LIKE 'weapons.%')`

	// One pass for everything this type did.
	if err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`units.type AS unit_type,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event IN ('land','runway_touch') THEN 1 ELSE 0 END) AS landings,
			SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'hit' AND `+notScenery+`
				AND NOT `+rammed+` THEN 1 ELSE 0 END) AS hits,
			SUM(CASE WHEN events.event = 'kill' AND `+notScenery+`
				AND NOT `+rammed+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('hit','kill') AND `+rammed+` THEN 1 ELSE 0 END) AS collisions,
			SUM(CASE WHEN events.event IN ('crash','dead','unit_lost','pilot_dead') THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Joins("LEFT JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Where("events.initiator_kind = ?", models.ObjectKindUnit).
		Group("units.type").
		Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate airframes: %v", err)
		return nil, err
	}

	// How often each was on the receiving end. Counted separately because it
	// keys on the target rather than the initiator, and a type can be shot
	// down having never fired a shot itself.
	var losses []struct {
		UnitType    string
		TimesKilled int
	}
	if err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("units.type AS unit_type, COUNT(*) AS times_killed").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units ON units.unit_id = targets.unit_id").
		Where("events.event = ? AND targets.kind = ?", "kill", models.ObjectKindUnit).
		Group("units.type").
		Scan(&losses).Error; err != nil {
		logs.Sugar.Errorf("Failed to count airframe losses: %v", err)
		return nil, err
	}

	byType := map[string]*models.AirframeStats{}
	result := make([]*models.AirframeStats, 0, len(rows))
	for i := range rows {
		byType[rows[i].UnitType] = &rows[i]
		result = append(result, &rows[i])
	}
	for _, l := range losses {
		if a := byType[l.UnitType]; a != nil {
			a.TimesKilled = l.TimesKilled
			continue
		}
		// Shot down without ever having done anything itself. It still flew.
		a := &models.AirframeStats{UnitType: l.UnitType, TimesKilled: l.TimesKilled}
		byType[l.UnitType] = a
		result = append(result, a)
	}

	return result, nil
}

// GetAirframeMatchups is the kill matrix: what each type shot down, and what
// shot it down, across every pilot on the server.
//
// The player page has this per pilot. Server-wide it answers a different
// question -- which airframe actually beats which -- and it is the closest
// thing the data has to a head-to-head record between machines.
func GetAirframeMatchups(missionID *uint) ([]*models.Matchup, error) {
	var rows []models.Matchup

	if err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("units.type AS unit_type, tunits.type AS target_type, COUNT(*) AS kills").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND targets.kind = ? AND events.initiator_kind = ?",
			"kill", models.ObjectKindUnit, models.ObjectKindUnit).
		Group("units.type, tunits.type").
		Order("kills DESC").
		Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate airframe matchups: %v", err)
		return nil, err
	}

	result := make([]*models.Matchup, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}
