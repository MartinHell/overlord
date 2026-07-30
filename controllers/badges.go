package controllers

import (
	"fmt"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// The badge shelf. Career-wide on purpose: a badge is something you keep, so
// unlike the rest of the dashboard these never narrow to one mission, though
// several are earned by what happened within a single one.
//
// Thresholds are chosen for a small server where one person and two AI sides
// fly. Tune them freely; the point is that every rule reads in one line.

// marksmanProgress maps the two gates onto one 0-100 bar: the sample size
// until twenty shots exist, then how much of the required ratio is reached.
func marksmanProgress(shots int, ratio float64) int {
	if shots < 20 {
		return shots * 100 / 20 / 2 // never past half while the sample is short
	}
	p := int(ratio / 0.5 * 100)
	if p > 100 {
		p = 100
	}
	return p
}

// GetBadges computes the shelf for one player.
func GetBadges(playerID uint) ([]*models.Badge, error) {
	db := initializers.DB

	// Kills per mission, non-scenery, which several badges read.
	var perMission []struct {
		MissionID uint
		Kills     int
	}
	if err := db.Model(&models.Event{}).
		Select("events.mission_id AS mission_id, COUNT(*) AS kills").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND events.player_id = ? AND targets.kind <> ? AND events.mission_id IS NOT NULL",
			"kill", playerID, models.ObjectKindScenery).
		Group("events.mission_id").
		Scan(&perMission).Error; err != nil {
		logs.Sugar.Errorf("Failed to count kills per mission for player %d: %v", playerID, err)
		return nil, err
	}

	bestKills, bestMission := 0, uint(0)
	for _, m := range perMission {
		if m.Kills > bestKills {
			bestKills, bestMission = m.Kills, m.MissionID
		}
	}

	// Whose was the first kill of each mission. MIN over the event id gives
	// one row per mission; the second query resolves those ids to players.
	var firstKillIDs []uint
	if err := db.Model(&models.Event{}).
		Select("MIN(events.id)").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND targets.kind <> ? AND events.mission_id IS NOT NULL",
			"kill", models.ObjectKindScenery).
		Group("events.mission_id").
		Scan(&firstKillIDs).Error; err != nil {
		logs.Sugar.Errorf("Failed to find first kills: %v", err)
		return nil, err
	}

	var firstBloods int64
	if len(firstKillIDs) > 0 {
		if err := db.Model(&models.Event{}).
			Where("events.id IN ? AND events.player_id = ?", firstKillIDs, playerID).
			Count(&firstBloods).Error; err != nil {
			logs.Sugar.Errorf("Failed to count first bloods for player %d: %v", playerID, err)
			return nil, err
		}
	}

	// Career totals in one pass.
	var totals struct {
		Shots, Kills, Takeoffs, Ejections int
	}
	if err := db.Model(&models.Event{}).
		Select(`SUM(CASE WHEN events.event = 'shot' THEN 1 ELSE 0 END) AS shots,
			SUM(CASE WHEN events.event = 'kill'
				AND (targets.kind IS NULL OR targets.kind <> 'scenery') THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS takeoffs,
			SUM(CASE WHEN events.event = 'ejection' THEN 1 ELSE 0 END) AS ejections`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ?", playerID).
		Scan(&totals).Error; err != nil {
		logs.Sugar.Errorf("Failed to total badge stats for player %d: %v", playerID, err)
		return nil, err
	}

	// Missions flown clean: five or more sorties, nothing lost.
	var cleanMissions []struct {
		MissionID uint
		Sorties   int
		Losses    int
	}
	if err := db.Model(&models.Event{}).
		Select(`events.mission_id AS mission_id,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties,
			SUM(CASE WHEN events.event IN ('crash','pilot_dead','ejection') THEN 1 ELSE 0 END) AS losses`).
		Where("events.player_id = ? AND events.mission_id IS NOT NULL", playerID).
		Group("events.mission_id").
		Scan(&cleanMissions).Error; err != nil {
		logs.Sugar.Errorf("Failed to check clean missions for player %d: %v", playerID, err)
		return nil, err
	}

	clean := false
	cleanBest := 0
	for _, m := range cleanMissions {
		if m.Sorties > cleanBest && m.Losses == 0 {
			cleanBest = m.Sorties
		}
		if m.Sorties >= 5 && m.Losses == 0 {
			clean = true
		}
	}

	// Carrier wire. Unvalidated against real data -- this server has recorded
	// no graded landings yet -- so the match is deliberately loose: an OK pass
	// mentioning wire 3, however DCS phrases it.
	var wires int64
	if err := db.Model(&models.Event{}).
		Where("events.event = ? AND events.player_id = ? AND events.comment LIKE ? AND events.comment LIKE ?",
			"landing_quality_mark", playerID, "%OK%", "%3%").
		Count(&wires).Error; err != nil {
		logs.Sugar.Errorf("Failed to count wires for player %d: %v", playerID, err)
		return nil, err
	}

	// Collateral, for the joke shelf.
	collateral, err := GetCollateral(&playerID, nil)
	if err != nil {
		return nil, err
	}

	marksmanRatio := 0.0
	if totals.Shots > 0 {
		marksmanRatio = float64(totals.Kills) / float64(totals.Shots)
	}

	badge := func(id, name, emoji, desc string, earned bool, progress, target int, detail string) *models.Badge {
		if progress > target {
			progress = target
		}
		return &models.Badge{
			ID: id, Name: name, Emoji: emoji, Description: desc,
			Earned: earned, Progress: progress, Target: target, Detail: detail,
		}
	}

	return []*models.Badge{
		badge("first-blood", "First Blood", "🩸",
			"Score the first kill of a mission.",
			firstBloods > 0, int(firstBloods), 1,
			fmt.Sprintf("First kill of the mission, %d time(s)", firstBloods)),
		badge("ace", "Ace", "🎖️",
			"Five kills in a single mission.",
			bestKills >= 5, bestKills, 5,
			fmt.Sprintf("%d kills in mission #%d", bestKills, bestMission)),
		badge("double-ace", "Double Ace", "🏅",
			"Ten kills in a single mission.",
			bestKills >= 10, bestKills, 10,
			fmt.Sprintf("%d kills in mission #%d", bestKills, bestMission)),
		// Progress tracks whichever gate is still closed: the sample size while
		// it is short, then the ratio. Tracking shots alone showed a full bar
		// at 20/20 while the badge stayed locked on a 0.22 ratio.
		badge("marksman", "Marksman", "🎯",
			"A kill for every other shot, over at least twenty shots.",
			totals.Shots >= 20 && marksmanRatio >= 0.5,
			marksmanProgress(totals.Shots, marksmanRatio), 100,
			fmt.Sprintf("%.2f kills per shot over %d shots", marksmanRatio, totals.Shots)),
		badge("three-wire", "OK 3-Wire", "🪝",
			"An OK-graded carrier pass on the three wire.",
			wires > 0, int(wires), 1,
			fmt.Sprintf("%d OK three-wire pass(es)", wires)),
		badge("clean-sheet", "Clean Sheet", "🛬",
			"Five sorties in one mission without losing an aircraft.",
			clean, cleanBest, 5,
			fmt.Sprintf("Best clean run: %d sorties", cleanBest)),
		badge("frequent-flyer", "Frequent Flyer", "✈️",
			"Twenty-five takeoffs, career.",
			totals.Takeoffs >= 25, totals.Takeoffs, 25,
			fmt.Sprintf("%d takeoffs", totals.Takeoffs)),
		badge("nylon-letdown", "Nylon Letdown", "🪂",
			"Ride the silk. Eject once.",
			totals.Ejections > 0, totals.Ejections, 1,
			fmt.Sprintf("%d ejection(s)", totals.Ejections)),
		badge("tree-trimmer", "Tree Trimmer", "🌲",
			"Catch one hundred trees in your blasts.",
			collateral.Trees >= 100, collateral.Trees, 100,
			fmt.Sprintf("%d trees and shrubs", collateral.Trees)),
		badge("urban-renewal", "Urban Renewal", "🧱",
			"Catch fifty walls, poles or buildings in your blasts.",
			collateral.Structures >= 50, collateral.Structures, 50,
			fmt.Sprintf("%d structures", collateral.Structures)),
	}, nil
}
