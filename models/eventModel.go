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
	PlayerID        uint
	Player          Player `gorm:"foreignKey:PlayerID;regerences:PlayerID"`
	Event           string
	InitiatorUnitID uint
	Initiator       Unit `gorm:"foreignKey:InitiatorUnitID;references:UnitID"`
	TargetID        uint
	Target          Unit `gorm:"foreignKey:TargetID;references:UnitID"`
	WeaponID        uint
	Weapon          Weapon `gorm:"foreignKey:WeaponID;references:WeaponID"`
}

func (e *Event) FromStreamEventsResponse(eventType string, p *Player, i *Unit, w *Weapon, t *Unit) {
	e.Event = eventType
	e.Player = *p
	e.Initiator = *i
	e.Weapon = *w
	if t != nil {
		e.Target = *t
	}
}

// CreatePlayer creates a player in the database
func (e *Event) CreateEvent() error {

	return initializers.DB.Transaction(func(tx *gorm.DB) error {
		// Check if the Player already exists, otherwise create it
		var player Player
		log.Printf("Player: %+v", e.Player)
		if err := tx.Where("uc_id = ?", e.Player.UCID).First(&player).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create the player if not found
				if err := tx.Create(&e.Player).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			log.Printf("PlayerID: %+v", player)
			e.PlayerID = player.PlayerID
		}

		// Check if the Initiator already exists, otherwise create it
		var initiator Unit
		if err := tx.Where("type = ?", e.Initiator.Type).First(&initiator).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create the initiator if not found
				if err := tx.Create(&e.Initiator).Error; err != nil {
					return err
				}
			}
		} else {
			e.InitiatorUnitID = initiator.UnitID
		}

		// Check if the Target already exists, otherwise create it
		if e.Target != (Unit{}) {
			var target Unit
			if err := tx.Where("type = ?", e.Target.Type).First(&target).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					// Create the target if not found
					if err := tx.Create(&e.Target).Error; err != nil {
						return err
					}
				}
			} else {
				e.TargetID = target.UnitID
			}
		}

		// Check if the Weapon already exists, otherwise create it
		var weapon Weapon
		if err := tx.Where("type = ?", e.Weapon.Type).First(&weapon).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create the weapon if not found
				if err := tx.Create(&e.Weapon).Error; err != nil {
					return err
				}
			}
		} else {
			e.WeaponID = weapon.WeaponID
		}

		// Create the event
		if err := tx.Create(&e).Error; err != nil {
			return err
		}

		return nil
	})

}
