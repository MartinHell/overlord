package controllers

import (
	"sort"
	"time"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// Sorties, derived from the event stream.
//
// A sortie is the span between a takeoff and whatever ended it. The events
// already say that exactly, in order, so this walks them rather than keeping a
// sorties table in step with them. See models.Sortie for the reasoning.
//
// Deliberately not offered for the synthetic AI players: their events are every
// AI unit on a coalition pooled under one id, so "the sortie between this
// takeoff and the next landing" would splice together aircraft that never met.

// maxSorties caps the log that reaches a page. Newest first, so the cap drops
// the oldest flights rather than the interesting ones.
const maxSorties = 40

// DCS reports one physical event several times over. A single departure arrives
// as runway_takeoff and then takeoff eleven seconds later; one arrival as
// runway_touch and then land, thirty-one seconds apart; one loss as pilot_dead,
// unit_lost and crash on the same timestamp. Taken literally that turns four
// flights into eight, half of them eleven seconds long and ending in "unknown".
//
// So repeats of a family inside a short window count as the one thing that
// happened. Nothing else can fall inside that window: a touch and go puts a
// departure between the two arrivals, and a departure ends any flight still
// open, so consecutive same-family reports really are duplicates.
const (
	famDepart = "depart"
	famArrive = "arrive"
	famEnd    = "end"
)

// mergeWindow generously covers the observed gaps of 11s and 31s.
const mergeWindow = 90.0

func sortieFamily(event string) string {
	switch event {
	case "takeoff", "runway_takeoff":
		return famDepart
	case "land", "runway_touch":
		return famArrive
	case "crash", "pilot_dead", "unit_lost", "dead", "ejection":
		return famEnd
	}
	return ""
}

// endOutcome ranks the several ways DCS describes one loss. Ejecting is the
// most specific thing that can be said about it -- the pilot got out -- and a
// crash says more than the bare fact of dying, so they win over the rest.
func endOutcome(event string) (string, int) {
	switch event {
	case "ejection":
		return models.SortieEjected, 3
	case "crash":
		return models.SortieCrashed, 2
	default:
		return models.SortieKilled, 1
	}
}

// sortieEvent is one row of the walk: the events that open, close or fill a
// flight, in the order DCS reported them.
type sortieEvent struct {
	ID          uint
	Event       string
	MissionID   *uint
	MissionTime float64
	UnitType    string
	CreatedAt   time.Time
}

// GetSorties reconstructs a player's flights, newest first.
func GetSorties(playerID uint, missionID *uint, unitType string) ([]*models.Sortie, error) {
	var player models.Player
	if err := initializers.DB.Where("player_id = ?", playerID).First(&player).Error; err != nil {
		logs.Sugar.Errorf("Failed to load player %d for sorties: %v", playerID, err)
		return nil, nil
	}
	if models.IsAIPlayerName(player.GetPlayerName()) {
		return nil, nil
	}

	var rows []sortieEvent
	if err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select(`events.id AS id,
			events.event AS event,
			events.mission_id AS mission_id,
			events.mission_time AS mission_time,
			units.type AS unit_type,
			events.created_at AS created_at`).
		Joins("LEFT JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.player_id = ? AND events.event IN ?", playerID, []string{
			"takeoff", "runway_takeoff",
			"land", "runway_touch",
			"crash", "pilot_dead", "ejection", "unit_lost", "dead",
			"kill",
		}).
		Where("events.event <> 'kill' OR "+notScenery).
		Order("events.id").
		Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to load sortie events for player %d: %v", playerID, err)
		return nil, err
	}

	missions, err := GetMissions()
	if err != nil {
		return nil, err
	}
	missionByID := map[uint]*models.MissionSummary{}
	for _, m := range missions {
		missionByID[m.MissionID] = m
	}

	var out []*models.Sortie
	var open *models.Sortie

	// close finishes the flight in hand. Called for every terminal event and
	// again when a fresh takeoff arrives with one still open, which is what a
	// missing landing looks like.
	close := func(outcome string, at float64) {
		if open == nil {
			return
		}
		if outcome != "" {
			open.Outcome = outcome
			open.Ended = true
			open.EndTime = at
			if at > open.StartTime {
				open.Duration = at - open.StartTime
			}
		}
		out = append(out, open)
		open = nil
	}

	// State for collapsing repeat reports of one event.
	lastFam := ""
	lastFamAt := 0.0
	var lastMission *uint
	endRank := 0

	sameEventAgain := func(fam string, r sortieEvent) bool {
		if fam != lastFam || fam == "" {
			return false
		}
		if r.MissionTime-lastFamAt > mergeWindow {
			return false
		}
		if (r.MissionID == nil) != (lastMission == nil) {
			return false
		}
		return r.MissionID == nil || *r.MissionID == *lastMission
	}

	for _, r := range rows {
		fam := sortieFamily(r.Event)

		if r.Event == "kill" {
			// Kills before the first takeoff belong to no flight -- a ground
			// unit, or a sortie whose takeoff went unrecorded.
			if open != nil {
				open.Kills++
			}
			continue
		}

		if sameEventAgain(fam, r) {
			// The same thing, said again. The only thing a repeat can still
			// tell us is a better word for how a flight ended.
			if fam == famEnd && len(out) > 0 {
				if name, rank := endOutcome(r.Event); rank > endRank {
					endRank = rank
					out[len(out)-1].Outcome = name
				}
			}
			continue
		}

		lastFam, lastFamAt, lastMission = fam, r.MissionTime, r.MissionID

		switch fam {
		case famDepart:
			// A departure with a flight still open means whatever ended the
			// last one was never recorded. Better an honest "unknown" than
			// folding two flights into one.
			close(models.SortieUnknown, r.MissionTime)
			endRank = 0

			s := &models.Sortie{
				UnitType:  r.UnitType,
				StartTime: r.MissionTime,
				Outcome:   models.SortieUnknown,
				StartedAt: r.CreatedAt,
			}
			if r.MissionID != nil {
				s.MissionID = *r.MissionID
				if m := missionByID[*r.MissionID]; m != nil {
					s.MissionName = m.Name
					s.Theatre = m.Theatre
				}
			}
			open = s

		case famArrive:
			close(models.SortieLanded, r.MissionTime)
			endRank = 0

		case famEnd:
			name, rank := endOutcome(r.Event)
			endRank = rank
			close(name, r.MissionTime)
		}
	}

	// A flight still open at the end of the events is exactly that: the pilot
	// is up, or the recording stopped before they came down. It is listed, not
	// invented an ending for.
	if open != nil {
		out = append(out, open)
	}

	// Newest first, and narrowed to one airframe when the page is.
	filtered := out[:0]
	for _, s := range out {
		if unitType != "" && s.UnitType != unitType {
			continue
		}
		filtered = append(filtered, s)
	}
	out = filtered

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MissionID != out[j].MissionID {
			return out[i].MissionID > out[j].MissionID
		}
		return out[i].StartTime > out[j].StartTime
	})

	if len(out) > maxSorties {
		out = out[:maxSorties]
	}

	return out, nil
}
