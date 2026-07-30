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
	// PlayerName is who the task or score belongs to as the mission spelled
	// it, empty for a mission-wide entry. The mission scripting environment
	// knows names and nothing else.
	PlayerName string `gorm:"index"`
	// PlayerID is the stable identity behind that name, resolved at ingest
	// from the live player list -- overlord already keys players by UCID, and
	// a mission publishes tasks about a pilot while that pilot is connected,
	// which is exactly the window the name is resolvable in. Null when the
	// name matched nobody; display falls back to the name.
	PlayerID *uint `gorm:"index"`
	Points   int
}
