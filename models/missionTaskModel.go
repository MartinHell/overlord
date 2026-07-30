package models

import "time"

// MissionTask is one task or score line exported by the running mission's own
// scripting, via the OVERLORD_EXPORT contract (see docs/mission-integration.md).
//
// The mission exports a snapshot, not an event log: every poll carries the
// current state of every task, and rows here are upserted by (mission, key).
// That makes the pipeline idempotent -- a missed poll costs nothing, a
// repeated one changes nothing -- and asks the mission scripter for the
// easiest thing to maintain: one table that reflects the present.
type MissionTask struct {
	ID        uint `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time

	MissionID uint   `gorm:"index;uniqueIndex:idx_mission_task_key"`
	TaskKey   string `gorm:"uniqueIndex:idx_mission_task_key"`

	Title string
	// State is the mission's own word for where the task stands: active, done,
	// failed. Free text on purpose; overlord displays it, it does not act on it.
	State string
	// PlayerName is who the task or score belongs to, empty for a side-wide
	// or mission-wide entry. Matched by name because the mission scripting
	// environment knows player names, not overlord's ids.
	PlayerName string `gorm:"index"`
	Points     int
}
