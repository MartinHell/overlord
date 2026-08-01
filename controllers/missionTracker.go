package controllers

import (
	"context"
	"sync"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/hook"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/world"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// The mission tracker decides which mission each incoming event belongs to.
//
// DCS does not number its runs, so the boundary has to be inferred. Two
// signals mark a new mission: an explicit mission_start event, and the mission
// clock jumping backwards. The clock is the one that matters in practice --
// out of fourteen recorded runs, only two ever produced a mission_start,
// because overlord is not always running when a mission loads.
type missionTracker struct {
	mu sync.Mutex
	// current is nil until the first event arrives, so an idle overlord does
	// not mint empty missions.
	current *uint
	// clock is the highest mission time seen in the current run.
	clock float64
	// resumed reports whether the tracker has tried to pick up where the
	// database left off after a restart.
	resumed bool
}

var tracker missionTracker

// missionResetSeconds is how far the clock must fall to count as a new run
// rather than jitter. Real resets drop by thousands of seconds; events within
// one run arrive at most a few seconds out of order.
const missionResetSeconds = 60

// MissionForEvent returns the mission id the event belongs to, opening a new
// mission when the event signals one.
func MissionForEvent(eventType string, missionTime float64) *uint {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if !tracker.resumed {
		tracker.resume()
	}

	if eventType == "mission_start" {
		tracker.open(missionTime)
	} else {
		tracker.observe(missionTime)
	}

	if missionTime > tracker.clock {
		tracker.clock = missionTime
	}

	return tracker.current
}

// MissionForClock returns the mission a freshly read mission clock belongs
// to, opening a new mission when the clock has gone backwards.
//
// This exists for the export poller. After a restart the poller can reach the
// new mission before any event does, and homing its rows by the last event's
// mission stamped a fresh run's tasks onto the dying seconds of the previous
// one -- worse, the upsert overwrote that run's final task states with the new
// run's "active". Reading the clock closes the race at its source.
func MissionForClock(missionTime float64) *uint {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if !tracker.resumed {
		tracker.resume()
	}

	tracker.observe(missionTime)

	if missionTime > tracker.clock {
		tracker.clock = missionTime
	}

	return tracker.current
}

// observe applies the clock-reset rule to a mission time from any source.
// Called under the lock.
func (t *missionTracker) observe(missionTime float64) {
	switch {
	case t.current == nil:
		t.open(missionTime)
	case missionTime > 0 && t.clock-missionTime > missionResetSeconds:
		// The clock went backwards: DCS restarted the mission while overlord
		// stayed up, or came back up after being down across a restart.
		t.open(missionTime)
	}
}

// CurrentMissionID reports the mission events are currently being tagged
// with, nil when nothing has arrived yet.
func CurrentMissionID() *uint {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if !tracker.resumed {
		tracker.resume()
	}

	return tracker.current
}

// resume adopts the newest mission already in the database, so an overlord
// restart mid-mission continues the run rather than splitting it. Called under
// the lock.
func (t *missionTracker) resume() {
	t.resumed = true

	var row struct {
		MissionID uint
		Clock     float64
	}

	err := initializers.DB.Model(&models.Event{}).
		Select("events.mission_id AS mission_id, MAX(events.mission_time) AS clock").
		Where("events.mission_id IS NOT NULL").
		Group("events.mission_id").
		Order("events.mission_id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		logs.Sugar.Errorf("Failed to resume mission tracking: %v", err)
		return
	}

	if row.MissionID != 0 {
		id := row.MissionID
		t.current = &id
		t.clock = row.Clock
		logs.Sugar.Infof("Resumed mission %d at %.0fs", id, row.Clock)

		// The resumed mission may predate name capture, or the fetch may have
		// failed last time; try again if it is still anonymous.
		go annotateMission(id, true)
	}
}

// open starts a new mission. Called under the lock.
func (t *missionTracker) open(missionTime float64) {
	mission := models.Mission{}
	if err := initializers.DB.Create(&mission).Error; err != nil {
		logs.Sugar.Errorf("Failed to create mission: %v", err)
		return
	}

	t.current = &mission.MissionID
	t.clock = missionTime
	logs.Sugar.Infof("Mission %d started (clock %.0fs)", mission.MissionID, missionTime)

	// A run appearing is the one change to the list worth showing before the
	// cache would have expired on its own -- it is what the dashboard watches
	// for to follow the server onto the new mission.
	invalidateMissions()

	go annotateMission(mission.MissionID, false)
}

// annotateMission asks DCS what the mission is called and where it plays, and
// writes what it learns onto the mission row. Best-effort by design: run as a
// goroutine, failing quietly, because knowing the name is worth one attempt
// and never worth blocking event ingestion.
func annotateMission(missionID uint, onlyIfEmpty bool) {
	if initializers.HookServiceClient == nil || initializers.WorldServiceClient == nil {
		return
	}

	if onlyIfEmpty {
		var existing models.Mission
		if err := initializers.DB.First(&existing, missionID).Error; err == nil &&
			(existing.Name != "" || existing.Theatre != "") {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updates := map[string]any{}

	if resp, err := initializers.HookServiceClient.GetMissionName(ctx, &hook.GetMissionNameRequest{}); err == nil {
		updates["name"] = resp.GetName()
	} else {
		logs.Sugar.Debugf("Could not fetch mission name: %v", err)
	}

	if resp, err := initializers.WorldServiceClient.GetTheatre(ctx, &world.GetTheatreRequest{}); err == nil {
		updates["theatre"] = resp.GetTheatre()
	} else {
		logs.Sugar.Debugf("Could not fetch theatre: %v", err)
	}

	if len(updates) == 0 {
		return
	}

	if err := initializers.DB.Model(&models.Mission{}).
		Where("mission_id = ?", missionID).
		Updates(updates).Error; err != nil {
		logs.Sugar.Errorf("Failed to annotate mission %d: %v", missionID, err)
		return
	}

	// The name arrives a moment after the mission does, over gRPC. Without this
	// the heading would read "Mission #48" for the rest of the TTL despite the
	// row already knowing better.
	invalidateMissions()
}
