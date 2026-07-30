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
	// MissionID ties the event to one run of a mission, so aggregates can be
	// scoped to "this mission" instead of all of recorded history. Nullable
	// because rows predate the missions table until the backfill has run.
	MissionID *uint `gorm:"index"`
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
	// Identity of the specific unit involved. Unit rows are deduplicated by
	// type, deliberately, so without these there is no way to tell one of four
	// F-16s from another, reconstruct a flight, or line an event up with a
	// track file.
	InitiatorName     string `gorm:"index"`
	InitiatorGroup    string `gorm:"index"`
	InitiatorCallsign string
	// Where the initiator was. Only captured at the moment of the event; see
	// the StreamUnits issue for continuous tracks.
	InitiatorLat float64
	InitiatorLon float64
	InitiatorAlt float64
	TargetID     *uint
	Target       Target `gorm:"foreignKey:TargetID;references:TargetID"`
	// TargetCoalition is stored on the event rather than on Target because
	// Target rows are deduplicated across events.
	TargetCoalition string `gorm:"index"`
	TargetName      string
	// Target position, which together with the initiator's gives engagement
	// geometry: range at launch, shot distance, and so on.
	TargetLat float64
	TargetLon float64
	TargetAlt float64
	WeaponID  *uint
	Weapon    Weapon `gorm:"foreignKey:WeaponID;references:WeaponID"`
	// Place is the airbase involved in takeoff, landing, engine and base
	// capture events.
	Place string `gorm:"index"`
	// Comment carries the landing grade on landingQualityMark events.
	Comment string
	// SlotID identifies the slot on playerChangeSlot events.
	SlotID string
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

		// Create the event. Built explicitly rather than reusing e so that the
		// preloaded association structs are not written back as new rows.
		event := Event{
			PlayerID:          e.PlayerID,
			Event:             e.Event,
			MissionID:         e.MissionID,
			MissionTime:       e.MissionTime,
			Coalition:         e.Coalition,
			InitiatorKind:     e.InitiatorKind,
			InitiatorUnitID:   e.InitiatorUnitID,
			InitiatorName:     e.InitiatorName,
			InitiatorGroup:    e.InitiatorGroup,
			InitiatorCallsign: e.InitiatorCallsign,
			InitiatorLat:      e.InitiatorLat,
			InitiatorLon:      e.InitiatorLon,
			InitiatorAlt:      e.InitiatorAlt,
			TargetID:          e.TargetID,
			TargetCoalition:   e.TargetCoalition,
			TargetName:        e.TargetName,
			TargetLat:         e.TargetLat,
			TargetLon:         e.TargetLon,
			TargetAlt:         e.TargetAlt,
			WeaponID:          e.WeaponID,
			Place:             e.Place,
			Comment:           e.Comment,
			SlotID:            e.SlotID,
		}

		logs.Sugar.Debugf("Creating Event: %+v", event)
		if err := tx.Create(&event).Error; err != nil {
			logs.Sugar.Errorf("Failed to create Event: %+v, error: %v", event, err)
			return err
		}

		return nil
	})
}
