package models

import (
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/common"
	"github.com/MartinHell/overlord/logs"
	"gorm.io/gorm"
)

type Unit struct {
	UnitID    uint `gorm:"primaryKey;autoIncrement;not null;unique;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	Type      string         `gorm:"not null;unique;index"`
}

func (u *Unit) FromCommonUnit(r *common.Unit) {
	u.Type = r.Type
}

func ensureUnit(tx *gorm.DB, unit Unit, unitType string) (*uint, error) {
	if unit.Type == "" {
		return nil, nil
	}

	var existingUnit Unit
	logs.Sugar.Debugf("Checking or creating %s with Type: %s", unitType, unit.Type)
	if err := tx.Where("type = ?", unit.Type).FirstOrCreate(&existingUnit, Unit{Type: unit.Type}).Error; err != nil {
		logs.Sugar.Errorf("Failed to find or create %s: %+v, error: %v", unitType, existingUnit, err)
		return nil, err
	}
	return &existingUnit.UnitID, nil
}
