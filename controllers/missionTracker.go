package controllers

import (
	"sync"

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

	switch {
	case eventType == "mission_start":
		tracker.open(missionTime)
	case tracker.current == nil:
		tracker.open(missionTime)
	case missionTime > 0 && tracker.clock-missionTime > missionResetSeconds:
		// The clock went backwards: DCS restarted the mission while overlord
		// stayed up, or came back up after being down across a restart.
		tracker.open(missionTime)
	}

	if missionTime > tracker.clock {
		tracker.clock = missionTime
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
}
