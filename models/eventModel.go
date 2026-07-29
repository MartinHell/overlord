package models

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
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
	PlayerID *uint
	Player   Player `gorm:"foreignKey:PlayerID;references:PlayerID"`
	Event    string
	// MissionTime is the DCS mission clock in seconds, as reported by the event
	// stream. CreatedAt records when overlord wrote the row, which drifts from
	// the sim and does not survive a restart mid-mission; this is the timestamp
	// that lines up with a track file.
	MissionTime float64 `gorm:"index"`
	// Coalition of the initiator at the time of the event. It lives here rather
	// than on Player because a human can switch sides between sorties, so only
	// the event knows which side they were on when it happened.
	Coalition       string `gorm:"index"`
	InitiatorUnitID *uint
	Initiator       Unit `gorm:"foreignKey:InitiatorUnitID;references:UnitID"`
	// InitiatorKind mirrors Target.Kind: an initiator can be a static, a weapon
	// or scenery, not just a unit, and its type is stored in Initiator either
	// way.
	InitiatorKind string `gorm:"index"`
	TargetID        *uint
	Target          Target `gorm:"foreignKey:TargetID;references:TargetID"`
	// TargetCoalition is stored on the event rather than on Target because
	// Target rows are deduplicated across events.
	TargetCoalition string `gorm:"index"`
	WeaponID        *uint
	Weapon          Weapon `gorm:"foreignKey:WeaponID;references:WeaponID"`
}

// Graphql structs used for pagination of events
type EventConnection struct {
	PageInfo *PageInfo    `json:"pageInfo"`
	Edges    []*EventEdge `json:"edges"`
}

type EventEdge struct {
	Node   *Event `json:"node"`
	Cursor string `json:"cursor"`
}

type PageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

func (e *Event) FromStreamEventsResponse(eventType string, p *Player, i *Unit, w *Weapon, t *Target) {
	e.Event = eventType
	if e.Coalition == "" {
		e.Coalition = CoalitionUnknown
	}
	if p != nil {
		e.Player = *p
	}
	if i.Type != "" {
		e.Initiator = *i
	}
	if w != nil {
		e.Weapon = *w
	}
	if t != nil {
		e.Target = *t
	}
}

// CreateEvent creates an event in the database
func (e *Event) CreateEvent() error {
	return initializers.DB.Transaction(func(tx *gorm.DB) error {
		var err error

		// Ensure Player exists or create it
		e.PlayerID, err = ensurePlayer(tx, e.Player)
		if err != nil {
			return err
		}

		// Ensure Initiator exists or create it
		e.InitiatorUnitID, err = ensureUnit(tx, e.Initiator)
		if err != nil {
			return err
		}

		// Ensure Target exists or create it if specified
		e.TargetID, err = ensureTarget(tx, e.Target)
		if err != nil {
			return err
		}

		// Ensure Weapon exists or create it
		e.WeaponID, err = ensureWeapon(tx, e.Weapon, "Weapon")
		if err != nil {
			return err
		}

		// Create the event
		event := Event{
			PlayerID:        e.PlayerID,
			Event:           e.Event,
			MissionTime:     e.MissionTime,
			Coalition:       e.Coalition,
			InitiatorKind:   e.InitiatorKind,
			InitiatorUnitID: e.InitiatorUnitID,
			TargetID:        e.TargetID,
			TargetCoalition: e.TargetCoalition,
			WeaponID:        e.WeaponID,
		}

		logs.Sugar.Debugf("Creating Event: %+v", event)
		if err := tx.Create(&event).Error; err != nil {
			logs.Sugar.Errorf("Failed to create Event: %+v, error: %v", event, err)
			return err
		}

		return nil
	})
}
