package controllers

import (
	"sort"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// The mission index: every recorded run with enough about it to choose one.
//
// A list of forty-three rows reading "Kolkhi_Test · Caucasus" tells you nothing
// about which night to open. Kills, who flew, and one standout moment do.
//
// Four queries and a pass over the kills in Go, rather than per-mission
// queries: forty-three missions would otherwise mean a hundred and thirty
// round trips to say the same thing.

// multiKillWindow is how close kills have to be to count as a burst. Matches
// the Hat Trick badge, so the page and the shelf agree about what a burst is.
const multiKillWindow = 60.0

// longShotNm is the range past which a shot is worth remarking on. Well short
// of the Long Shot badge's fifteen, because this is the best of one mission
// rather than a career achievement.
const longShotNm = 8.0

// killFact is one kill, reduced to what a highlight needs.
type killFact struct {
	MissionID    uint
	PlayerID     uint
	PlayerName   string
	MissionTime  float64
	InitiatorLat float64
	InitiatorLon float64
	TargetLat    float64
	TargetLon    float64
}

// GetMissionIndex lists every recorded mission, newest first, with the extra
// detail the index page shows.
func GetMissionIndex() ([]*models.MissionEntry, error) {
	base, err := GetMissions()
	if err != nil {
		return nil, err
	}

	entries := make([]*models.MissionEntry, 0, len(base))
	byID := map[uint]*models.MissionEntry{}
	for _, m := range base {
		e := &models.MissionEntry{MissionSummary: *m}
		entries = append(entries, e)
		byID[m.MissionID] = e
	}
	if len(entries) == 0 {
		return entries, nil
	}

	// Kills and sorties per mission.
	var totals []struct {
		MissionID uint
		Kills     int
		Sorties   int
	}
	if err := initializers.DB.Model(&models.Event{}).
		Select(`events.mission_id AS mission_id,
			SUM(CASE WHEN events.event = 'kill' AND `+notScenery+` THEN 1 ELSE 0 END) AS kills,
			SUM(CASE WHEN events.event IN ('takeoff','runway_takeoff') THEN 1 ELSE 0 END) AS sorties`).
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.mission_id IS NOT NULL").
		Group("events.mission_id").
		Scan(&totals).Error; err != nil {
		logs.Sugar.Errorf("Failed to total missions: %v", err)
		return nil, err
	}
	for _, t := range totals {
		if e := byID[t.MissionID]; e != nil {
			e.Kills = t.Kills
			e.Sorties = t.Sorties
		}
	}

	// Which humans flew each mission, and what they got. AI is left out: it is
	// in every mission, so listing it distinguishes none of them.
	var pilots []struct {
		MissionID  uint
		PlayerID   uint
		PlayerName string
		Kills      int
	}
	if err := initializers.DB.Model(&models.Event{}).
		Select(`events.mission_id AS mission_id,
			players.player_id AS player_id,
			players.player_name AS player_name,
			SUM(CASE WHEN events.event = 'kill' AND `+notScenery+` THEN 1 ELSE 0 END) AS kills`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.mission_id IS NOT NULL AND players.player_name NOT LIKE ?", "AI-Unit%").
		Group("events.mission_id, players.player_id, players.player_name").
		Scan(&pilots).Error; err != nil {
		logs.Sugar.Errorf("Failed to list mission pilots: %v", err)
		return nil, err
	}
	for _, p := range pilots {
		if e := byID[p.MissionID]; e != nil {
			e.Pilots = append(e.Pilots, &models.MissionPilot{
				PlayerID: p.PlayerID, PlayerName: p.PlayerName, Kills: p.Kills,
			})
		}
	}
	for _, e := range entries {
		sort.SliceStable(e.Pilots, func(i, j int) bool {
			if e.Pilots[i].Kills != e.Pilots[j].Kills {
				return e.Pilots[i].Kills > e.Pilots[j].Kills
			}
			return e.Pilots[i].PlayerName < e.Pilots[j].PlayerName
		})
	}

	// Every kill, once, for the highlights. One scan beats a query per mission,
	// and the row is small enough that the whole history fits comfortably.
	var kills []killFact
	if err := initializers.DB.Model(&models.Event{}).
		Select(`events.mission_id AS mission_id,
			events.player_id AS player_id,
			players.player_name AS player_name,
			events.mission_time AS mission_time,
			events.initiator_lat AS initiator_lat,
			events.initiator_lon AS initiator_lon,
			events.target_lat AS target_lat,
			events.target_lon AS target_lon`).
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND events.mission_id IS NOT NULL AND "+notScenery, "kill").
		Order("events.mission_id, events.id").
		Scan(&kills).Error; err != nil {
		logs.Sugar.Errorf("Failed to load kills for mission highlights: %v", err)
		return nil, err
	}

	perMission := map[uint][]killFact{}
	for _, k := range kills {
		perMission[k.MissionID] = append(perMission[k.MissionID], k)
	}
	for id, ks := range perMission {
		if e := byID[id]; e != nil {
			e.Highlight = highlightOf(ks)
		}
	}

	return entries, nil
}

// highlightOf picks the one thing worth saying about a mission.
//
// Humans win ties they have no business winning: a night where a real pilot
// took two aircraft is a better story than one where an AI coalition took
// ninety, because the AI does that every night. Only when no human did
// anything at all does the AI's tally get the line.
func highlightOf(kills []killFact) *models.MissionHighlight {
	if len(kills) == 0 {
		return nil
	}

	human := make([]killFact, 0, len(kills))
	for _, k := range kills {
		if !models.IsAIPlayerName(k.PlayerName) {
			human = append(human, k)
		}
	}

	if h := bestHighlight(human, true); h != nil {
		return h
	}
	return bestHighlight(kills, false)
}

// bestHighlight finds the most interesting thing in one set of kills, by the
// order the Highlight kinds are declared in.
//
// asPilot says whether these kills belong to one person. Only then do bursts
// and totals mean anything: the synthetic AI players pool a whole coalition, so
// "twelve kills in twenty-four seconds" is the entire blue air force having a
// busy minute rather than somebody's moment. Allowing it made thirty-seven of
// forty-three missions read the same way, which is the opposite of telling them
// apart. A long shot survives, because a single missile flying fifteen miles is
// one event no matter who is credited with it.
func bestHighlight(kills []killFact, asPilot bool) *models.MissionHighlight {
	if len(kills) == 0 {
		return nil
	}

	byPlayer := map[uint][]killFact{}
	for _, k := range kills {
		byPlayer[k.PlayerID] = append(byPlayer[k.PlayerID], k)
	}

	var burst, ace, shot, top *models.MissionHighlight

	for id, ks := range byPlayer {
		pid := id
		name := ks[0].PlayerName

		if asPilot {
			// A burst: the most kills any window of sixty seconds holds.
			timed := make([]float64, 0, len(ks))
			for _, k := range ks {
				if k.MissionTime > 0 {
					timed = append(timed, k.MissionTime)
				}
			}
			sort.Float64s(timed)
			for i := 0; i < len(timed); i++ {
				j := i
				for j+1 < len(timed) && timed[j+1]-timed[i] <= multiKillWindow {
					j++
				}
				n := j - i + 1
				if n >= 3 && (burst == nil || n > burst.Count) {
					burst = &models.MissionHighlight{
						Kind: models.HighlightMultiKill, PlayerID: &pid, PlayerName: name,
						Count: n, Seconds: timed[j] - timed[i],
					}
				}
			}

			if len(ks) >= 5 && (ace == nil || len(ks) > ace.Count) {
				ace = &models.MissionHighlight{
					Kind: models.HighlightAce, PlayerID: &pid, PlayerName: name, Count: len(ks),
				}
			}
		}

		for _, k := range ks {
			if k.InitiatorLat == 0 || k.TargetLat == 0 {
				continue
			}
			nm := haversineM(k.InitiatorLat, k.InitiatorLon, k.TargetLat, k.TargetLon) / 1852
			if nm >= longShotNm && (shot == nil || nm > shot.Nm) {
				shot = &models.MissionHighlight{
					Kind: models.HighlightLongShot, PlayerID: &pid, PlayerName: name, Nm: nm,
				}
			}
		}

		if asPilot && len(ks) >= 2 && (top == nil || len(ks) > top.Count) {
			top = &models.MissionHighlight{
				Kind: models.HighlightTopScorer, PlayerID: &pid, PlayerName: name, Count: len(ks),
			}
		}
	}

	for _, h := range []*models.MissionHighlight{burst, ace, shot, top} {
		if h != nil {
			return h
		}
	}
	return nil
}
