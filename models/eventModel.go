package models

import (
	"log"

	"github.com/MartinHell/overlord/initializers"
	"gorm.io/gorm"
)

/* type Event struct {
	gorm.Model
	PlayerID uint   `json:"playerid"`
	Event    string `json:"event"`
	TargetID *uint  `json:"targetid"`
	WeaponID *uint  `json:"weaponid"`
} */

type Event struct {
	gorm.Model
	PlayerID        *uint
	Player          Player `gorm:"foreignKey:PlayerID;references:PlayerID"`
	Event           string
	InitiatorUnitID *uint
	Initiator       Unit `gorm:"foreignKey:InitiatorUnitID;references:UnitID"`
	TargetID        *uint
	Target          Unit `gorm:"foreignKey:TargetID;references:UnitID"`
	WeaponID        *uint
	Weapon          Weapon `gorm:"foreignKey:WeaponID;references:WeaponID"`
	TargetWeaponID  *uint
	TargetWeapon    Weapon `gorm:"foreignKey:TargetWeaponID;references:WeaponID"`
}

func (e *Event) FromStreamEventsResponse(eventType string, p *Player, i *Unit, w *Weapon, t *Unit, tw *Weapon) {
	e.Event = eventType
	e.Player = *p
	e.Initiator = *i
	e.Weapon = *w
	if t != nil {
		e.Target = *t
	}
	if tw != nil {
		e.TargetWeapon = *tw
	}
}

// CreateEvent creates an event in the database
func (e *Event) CreateEvent() error {
	return initializers.DB.Transaction(func(tx *gorm.DB) error {
		// Ensure Player exists or create it
		if e.Player.UCID != "" {
			var player Player
			log.Printf("Checking or creating Player with UCID: %s", e.Player.UCID)
			if err := tx.Where("uc_id = ?", e.Player.UCID).FirstOrCreate(&player, Player{UCID: e.Player.UCID, PlayerName: e.Player.PlayerName}).Error; err != nil {
				log.Printf("Failed to find or create Player: %+v, error: %v", player, err)
				return err
			}
			e.PlayerID = &player.PlayerID
		} else {
			e.PlayerID = nil
		}

		// Ensure Initiator exists or create it
		if e.Initiator.Type != "" {
			var initiator Unit
			log.Printf("Checking or creating Initiator with Type: %s", e.Initiator.Type)
			if err := tx.Where("type = ?", e.Initiator.Type).FirstOrCreate(&initiator, Unit{Type: e.Initiator.Type}).Error; err != nil {
				log.Printf("Failed to find or create Initiator: %+v, error: %v", initiator, err)
				return err
			}
			e.InitiatorUnitID = &initiator.UnitID
		} else {
			e.InitiatorUnitID = nil
		}

		// Ensure Target exists or create it if specified
		if e.Target.Type != "" {
			var target Unit
			log.Printf("Checking or creating Target with Type: %s", e.Target.Type)
			if err := tx.Where("type = ?", e.Target.Type).FirstOrCreate(&target, Unit{Type: e.Target.Type}).Error; err != nil {
				log.Printf("Failed to find or create Target: %+v, error: %v", target, err)
				return err
			}
			e.TargetID = &target.UnitID
		} else {
			e.TargetID = nil
		}

		// Ensure TargetWeapon exists or create it if specified
		if e.TargetWeapon.Type != "" {
			var targetWeapon Weapon
			log.Printf("Checking or creating TargetWeapon with Type: %s", e.TargetWeapon.Type)
			if err := tx.Where("type = ?", e.TargetWeapon.Type).FirstOrCreate(&targetWeapon, Weapon{Type: e.TargetWeapon.Type}).Error; err != nil {
				log.Printf("Failed to find or create TargetWeapon: %+v, error: %v", targetWeapon, err)
				return err
			}
			e.TargetWeaponID = &targetWeapon.WeaponID
		} else {
			e.TargetWeaponID = nil
		}

		// Ensure Weapon exists or create it
		if e.Weapon.Type != "" {
			var weapon Weapon
			log.Printf("Checking or creating Weapon with Type: %s", e.Weapon.Type)
			if err := tx.Where("type = ?", e.Weapon.Type).FirstOrCreate(&weapon, Weapon{Type: e.Weapon.Type}).Error; err != nil {
				log.Printf("Failed to find or create Weapon: %+v, error: %v", weapon, err)
				return err
			}
			e.WeaponID = &weapon.WeaponID
		} else {
			e.WeaponID = nil
		}

		// Create the event
		event := Event{
			PlayerID:        e.PlayerID,
			Event:           e.Event,
			InitiatorUnitID: e.InitiatorUnitID,
			TargetID:        e.TargetID,
			WeaponID:        e.WeaponID,
			TargetWeaponID:  e.TargetWeaponID,
		}

		log.Printf("Creating Event: %+v", event)
		if err := tx.Create(&event).Error; err != nil {
			log.Printf("Failed to create Event: %+v, error: %v", event, err)
			return err
		}

		return nil
	})
}
