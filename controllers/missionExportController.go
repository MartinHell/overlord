package controllers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/custom"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/timer"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"gorm.io/gorm/clause"
)

// The mission export poller.
//
// The running mission maintains a plain Lua table, OVERLORD_EXPORT, with its
// tasks and scores; this loop asks for it over CustomService.Eval and upserts
// the result. Polling a snapshot, rather than having the mission push events,
// is a deliberate trade: it costs a small query every few seconds and buys an
// idempotent pipeline where a missed poll loses nothing and the mission-side
// contract is one table that reflects the present.
//
// Eval is arbitrary Lua execution inside the mission and is disabled in
// DCS-gRPC by default. Overlord only ever sends the one statement below; the
// off switch lives on the DCS side (evalEnabled in dcs-grpc.lua).

const (
	exportPollInterval = 10 * time.Second
	// exportBackoff is how long to wait after a failure before trying again.
	// The common failure is eval being disabled or the mission not defining
	// the table, neither of which resolves in ten seconds, and neither of
	// which deserves a log line every ten seconds either.
	exportBackoff = 5 * time.Minute

	exportLua = "return OVERLORD_EXPORT"
)

// missionExport is the wire shape of OVERLORD_EXPORT; see
// docs/mission-integration.md for the contract the mission side implements.
type missionExport struct {
	Version int `json:"version"`
	Tasks   []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Player string `json:"player"`
		Points int    `json:"points"`
	} `json:"tasks"`
}

// PollMissionExport runs forever, keeping the mission_tasks table in step with
// the running mission's export. Meant to be started as a goroutine next to the
// event stream.
func PollMissionExport(ctx context.Context) {
	logged := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(exportPollInterval):
		}

		if err := pullMissionExport(ctx); err != nil {
			if !logged {
				logs.Sugar.Infof("Mission export not available (%v); retrying every %s", err, exportBackoff)
				logged = true
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(exportBackoff - exportPollInterval):
			}
			continue
		}

		if logged {
			logs.Sugar.Info("Mission export is flowing again")
			logged = false
		}
	}
}

func pullMissionExport(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := initializers.CustomServiceClient.Eval(callCtx, &custom.EvalRequest{Lua: exportLua})
	if err != nil {
		return err
	}

	// A mission that defines no OVERLORD_EXPORT returns null. That is not an
	// error, it is a mission with nothing to say.
	raw := resp.GetJson()
	if raw == "" || raw == "null" {
		return nil
	}

	var export missionExport
	if err := json.Unmarshal([]byte(raw), &export); err != nil {
		logs.Sugar.Warnf("Mission export is not valid JSON for the contract: %v", err)
		return nil // malformed content is the mission's bug, not a reason to back off
	}

	// Home the rows by the mission clock, not by the last event: after a
	// restart this poller can beat the first event to the new mission, and
	// homing by event stamped a fresh run's tasks onto the previous one.
	mission := CurrentMissionID()
	if t, err := initializers.TimerServiceClient.GetTime(callCtx, &timer.GetTimeRequest{}); err == nil {
		mission = MissionForClock(t.GetTime())
	}
	if mission == nil {
		return nil
	}

	// Resolve each exported name to a stable player once per poll. The name
	// is what the mission knows; the identity behind it is what survives a
	// rename between sessions.
	resolved := map[string]*uint{}
	resolve := func(name string) *uint {
		if name == "" {
			return nil
		}
		if id, seen := resolved[name]; seen {
			return id
		}
		var player models.Player
		player.PlayerName = &name
		if err := player.GetPlayerFromDB(); err == nil && player.PlayerID != 0 {
			id := player.PlayerID
			resolved[name] = &id
			return &id
		}
		resolved[name] = nil
		return nil
	}

	for _, t := range export.Tasks {
		if t.ID == "" {
			continue
		}

		row := models.MissionTask{
			MissionID:  *mission,
			TaskKey:    t.ID,
			Title:      t.Title,
			State:      t.State,
			PlayerName: t.Player,
			PlayerID:   resolve(t.Player),
			Points:     t.Points,
		}

		// Upsert on (mission, key): the export is a snapshot, so the newest
		// poll simply wins.
		if err := initializers.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "mission_id"}, {Name: "task_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "state", "player_name", "player_id", "points", "updated_at"}),
		}).Create(&row).Error; err != nil {
			logs.Sugar.Errorf("Failed to upsert mission task %q: %v", t.ID, err)
		}
	}

	return nil
}

// GetMissionTasks lists the exported tasks, scoped like everything else.
func GetMissionTasks(missionID *uint) ([]*models.MissionTask, error) {
	var rows []*models.MissionTask

	q := initializers.DB.Model(&models.MissionTask{}).Order("points DESC, task_key")
	if missionID != nil {
		q = q.Where("mission_id = ?", *missionID)
	}

	if err := q.Find(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to list mission tasks: %v", err)
		return nil, err
	}

	return rows, nil
}
