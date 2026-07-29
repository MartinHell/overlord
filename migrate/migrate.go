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

	if err := initializers.DB.AutoMigrate(
		&models.Player{},
		&models.Unit{},
		&models.Weapon{},
		&models.Target{},
		&models.Event{},
	); err != nil {
		logs.Sugar.Fatalf("Migration failed: %v", err)
	}

	logs.Sugar.Infoln("Migration complete")
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
