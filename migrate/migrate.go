package main

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
	"gorm.io/gorm"
)

func init() {
	initializers.LoadEnvVariables()
	initializers.ConnectToDB()
}

func main() {
	if err := dedupeWeapons(initializers.DB); err != nil {
		logs.Sugar.Fatalf("Failed to deduplicate weapons: %v", err)
	}

	if err := dropPlayerIP(initializers.DB); err != nil {
		logs.Sugar.Fatalf("Failed to drop the player IP column: %v", err)
	}

	if err := initializers.DB.AutoMigrate(
		&models.Player{},
		&models.Unit{},
		&models.Weapon{},
		&models.Target{},
		&models.Event{},
		&models.Mission{},
		&models.MissionTask{},
	); err != nil {
		logs.Sugar.Fatalf("Migration failed: %v", err)
	}

	if err := backfillMissions(initializers.DB); err != nil {
		logs.Sugar.Fatalf("Failed to backfill missions: %v", err)
	}

	logs.Sugar.Infoln("Migration complete")
}

// backfillMissions segments the events recorded before the missions table
// existed into runs, using the same signals as live ingestion: an explicit
// mission_start, or the mission clock falling. Only rows with no mission are
// touched, so this is safe to run again.
func backfillMissions(db *gorm.DB) error {
	var pending int64
	if err := db.Model(&models.Event{}).Where("mission_id IS NULL").Count(&pending).Error; err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}

	type row struct {
		ID          uint
		Event       string
		MissionTime float64
	}

	var events []row
	if err := db.Model(&models.Event{}).
		Select("events.id, events.event, events.mission_time").
		Where("events.mission_id IS NULL").
		Order("events.id").
		Scan(&events).Error; err != nil {
		return err
	}

	// A segment is a run of consecutive event ids that belong to one mission.
	type segment struct{ first, last uint }
	var segments []segment
	clock := 0.0

	for _, e := range events {
		newRun := len(segments) == 0 ||
			e.Event == "mission_start" ||
			(e.MissionTime > 0 && clock-e.MissionTime > 60)

		if newRun {
			segments = append(segments, segment{first: e.ID, last: e.ID})
			clock = e.MissionTime
			continue
		}

		segments[len(segments)-1].last = e.ID
		if e.MissionTime > clock {
			clock = e.MissionTime
		}
	}

	for _, seg := range segments {
		mission := models.Mission{}
		if err := db.Create(&mission).Error; err != nil {
			return err
		}

		if err := db.Model(&models.Event{}).
			Where("id >= ? AND id <= ? AND mission_id IS NULL", seg.first, seg.last).
			Update("mission_id", mission.MissionID).Error; err != nil {
			return err
		}
	}

	logs.Sugar.Infof("Backfilled %d events into %d missions", pending, len(segments))
	return nil
}

// dropPlayerIP removes the players.ip column and any addresses already stored
// in it. AutoMigrate never drops columns, so without this the data would linger
// in every existing database even though nothing writes or reads it any more.
//
// IP addresses are personal data, overlord had no use for them, and the field
// was readable over an unauthenticated API. Not collecting them is simpler than
// protecting them.
func dropPlayerIP(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Player{}) {
		return nil
	}

	if !db.Migrator().HasColumn(&models.Player{}, "ip") {
		return nil
	}

	logs.Sugar.Infoln("Dropping the players.ip column and the addresses in it")

	return db.Migrator().DropColumn(&models.Player{}, "ip")
}

// dedupeWeapons collapses duplicate weapon rows before the unique index on
// Weapon.Type is added, since AutoMigrate cannot create that index while
// duplicates exist. Duplicates were possible because ensureWeapon uses
// FirstOrCreate, which is not atomic without the constraint.
//
// The lowest ID for each type wins, and every event and target pointing at a
// loser is repointed at the winner.
func dedupeWeapons(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Weapon{}) {
		return nil
	}

	type duplicate struct {
		KeepID uint
		Type   string
	}

	var duplicates []duplicate
	if err := db.Model(&models.Weapon{}).
		Select("MIN(weapon_id) AS keep_id, type").
		Group("type").
		Having("COUNT(*) > 1").
		Scan(&duplicates).Error; err != nil {
		return err
	}

	if len(duplicates) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, dup := range duplicates {
			var losers []uint
			if err := tx.Model(&models.Weapon{}).
				Where("type = ? AND weapon_id <> ?", dup.Type, dup.KeepID).
				Pluck("weapon_id", &losers).Error; err != nil {
				return err
			}

			logs.Sugar.Infof("Collapsing %d duplicate rows for weapon %q into ID %d", len(losers), dup.Type, dup.KeepID)

			if err := tx.Model(&models.Event{}).
				Where("weapon_id IN ?", losers).
				Update("weapon_id", dup.KeepID).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.Target{}).
				Where("weapon_id IN ?", losers).
				Update("weapon_id", dup.KeepID).Error; err != nil {
				return err
			}

			if err := tx.Unscoped().Where("weapon_id IN ?", losers).Delete(&models.Weapon{}).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
