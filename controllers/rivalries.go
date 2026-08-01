package controllers

import (
	"sort"

	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// Head-to-head records and airframe titles.
//
// Both lean on the same trick the nemesis dossier line uses: a kill event names
// the victim's DCS unit rather than its owner, so a victim is tied back to a
// pilot by matching the unit name against the names that pilot has been seen
// flying. Exact within a mission, which is the granularity that matters -- unit
// names are reused across missions, so this is deliberately not a claim about
// history.

// GetRivalries returns this player's record against everyone they have traded
// kills with, best-known opponent first.
//
// Done in two scoped steps rather than one join. The obvious query -- build a
// name-to-owner map over every event and join kills against it -- is a nested
// loop over two unindexed name columns: measured at 79 seconds on this
// database, long enough to collide with live ingestion and fail on
// SQLITE_BUSY. Asking for one player's victims first bounds everything after
// it to a handful of names.
func GetRivalries(playerID uint, missionID *uint) ([]*models.Rivalry, error) {
	db := initializers.DB

	var player models.Player
	if err := db.Where("player_id = ?", playerID).First(&player).Error; err != nil {
		return nil, nil
	}
	// The synthetic AI players pool a coalition's every unit under one id, so
	// "their record against you" is the whole war and takes as long to compute.
	if models.IsAIPlayerName(player.GetPlayerName()) {
		return nil, nil
	}

	type tally struct {
		ID    uint
		Name  string
		Total int
	}

	// Step one: what this player shot down, by the victim's unit name.
	var victims []struct {
		Name  string
		Total int
	}
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("events.target_name AS name, COUNT(*) AS total").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND events.player_id = ? AND events.target_name <> ? AND "+notScenery,
			"kill", playerID, "").
		Group("events.target_name").
		Scan(&victims).Error; err != nil {
		logs.Sugar.Errorf("Failed to list victims of player %d: %v", playerID, err)
		return nil, err
	}

	// Step two: who was flying those. One pass, bounded by the names above.
	var scored []tally
	if len(victims) > 0 {
		names := make([]string, 0, len(victims))
		for _, v := range victims {
			names = append(names, v.Name)
		}

		var owners []struct {
			Name     string
			PlayerID uint
			Owner    string
		}
		if err := db.Model(&models.Event{}).
			Scopes(scopeMission(missionID)).
			Select(`DISTINCT events.initiator_name AS name,
				events.player_id AS player_id,
				players.player_name AS owner`).
			Joins("JOIN players ON players.player_id = events.player_id").
			Where("events.initiator_name IN ?", names).
			Scan(&owners).Error; err != nil {
			logs.Sugar.Errorf("Failed to resolve victim owners for player %d: %v", playerID, err)
			return nil, err
		}

		// First owner wins. DCS reuses unit names across missions, so a name
		// can resolve to more than one pilot over history; counting both would
		// credit the same kill twice.
		ownerOf := map[string]struct {
			ID   uint
			Name string
		}{}
		for _, o := range owners {
			if _, seen := ownerOf[o.Name]; !seen {
				ownerOf[o.Name] = struct {
					ID   uint
					Name string
				}{o.PlayerID, o.Owner}
			}
		}

		totals := map[uint]*tally{}
		for _, v := range victims {
			own, ok := ownerOf[v.Name]
			if !ok || own.ID == playerID {
				continue
			}
			t := totals[own.ID]
			if t == nil {
				t = &tally{ID: own.ID, Name: own.Name}
				totals[own.ID] = t
			}
			t.Total += v.Total
		}
		for _, t := range totals {
			scored = append(scored, *t)
		}
	}

	// What was shot down out from under them, and by whom.
	mine := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("DISTINCT events.initiator_name").
		Where("events.player_id = ? AND events.initiator_name <> ?", playerID, "")

	var conceded []tally
	if err := db.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("players.player_id AS id, players.player_name AS name, COUNT(*) AS total").
		Joins("JOIN players ON players.player_id = events.player_id").
		Where("events.event = ? AND events.player_id <> ? AND events.target_name IN (?)",
			"kill", playerID, mine).
		Group("players.player_id, players.player_name").
		Scan(&conceded).Error; err != nil {
		logs.Sugar.Errorf("Failed to tally kills against player %d: %v", playerID, err)
		return nil, err
	}

	byID := map[uint]*models.Rivalry{}
	get := func(t tally) *models.Rivalry {
		r := byID[t.ID]
		if r == nil {
			id := t.ID
			r = &models.Rivalry{
				OpponentID:   &id,
				OpponentName: t.Name,
				IsAI:         models.IsAIPlayerName(t.Name),
			}
			byID[t.ID] = r
		}
		return r
	}
	for _, t := range scored {
		get(t).Killed = t.Total
	}
	for _, t := range conceded {
		get(t).Lost = t.Total
	}

	out := make([]*models.Rivalry, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}

	// People before machines, then by how much has passed between them: a
	// rivalry is measured by how often you have met, not by who is winning.
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsAI != out[j].IsAI {
			return !out[i].IsAI
		}
		li, lj := out[i].Killed+out[i].Lost, out[j].Killed+out[j].Lost
		if li != lj {
			return li > lj
		}
		return out[i].OpponentName < out[j].OpponentName
	})

	return out, nil
}

// GetTitles returns the airframes on which this player holds the server record.
//
// Career-wide whatever the scope says, for the same reason badges are: a title
// is something you hold until somebody takes it, not something that resets when
// the mission does. Humans only -- the AI pools a coalition's every kill under
// one id and would hold every title forever.
func GetTitles(playerID uint) ([]*models.Title, error) {
	var rows []struct {
		UnitType string
		PlayerID uint
		Name     string
		Kills    int
	}

	if err := initializers.DB.Model(&models.Event{}).
		Select(`units.type AS unit_type,
			events.player_id AS player_id,
			players.player_name AS name,
			COUNT(*) AS kills`).
		Joins("JOIN units ON units.unit_id = events.initiator_unit_id").
		Joins("JOIN players ON players.player_id = events.player_id").
		Joins("LEFT JOIN targets ON targets.target_id = events.target_id").
		Where("events.event = ? AND "+notScenery, "kill").
		Group("units.type, events.player_id, players.player_name").
		Order("units.type, kills DESC, players.player_name").
		Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to rank airframe titles: %v", err)
		return nil, err
	}

	// Rows arrive grouped by airframe with the leader first, so the first
	// human seen for a type is that type's title holder. A tie goes to the
	// name that sorts first, which is arbitrary but stable -- a title that
	// changed hands on every page load would be worse than an unfair one.
	var out []*models.Title
	seen := map[string]bool{}
	for _, r := range rows {
		if models.IsAIPlayerName(r.Name) || seen[r.UnitType] || r.Kills <= 0 {
			continue
		}
		seen[r.UnitType] = true
		if r.PlayerID == playerID {
			out = append(out, &models.Title{UnitType: r.UnitType, Kills: r.Kills})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kills != out[j].Kills {
			return out[i].Kills > out[j].Kills
		}
		return out[i].UnitType < out[j].UnitType
	})

	return out, nil
}
