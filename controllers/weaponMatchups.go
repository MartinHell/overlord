package controllers

import (
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/models"
)

// What each store actually kills.
//
// The weapons table says how often a weapon hits and how often that ends in a
// kill. It does not say what it kills, which is the difference between a
// number and a picture: an AGM-154 with 288 kills is a different weapon
// depending on whether they are tanks or radars.

// GetWeaponMatchups totals kills per weapon against target type.
func GetWeaponMatchups(missionID *uint) ([]*models.WeaponMatchup, error) {
	var rows []models.WeaponMatchup

	if err := initializers.DB.Model(&models.Event{}).
		Scopes(scopeMission(missionID)).
		Select("weapons.type AS weapon_type, tunits.type AS target_type, COUNT(*) AS kills").
		Joins("JOIN weapons ON weapons.weapon_id = events.weapon_id").
		Joins("JOIN targets ON targets.target_id = events.target_id").
		Joins("JOIN units AS tunits ON tunits.unit_id = targets.unit_id").
		Joins("LEFT JOIN units AS iunits ON iunits.unit_id = events.initiator_unit_id").
		Where("events.event = ? AND targets.kind <> ? AND NOT "+isCollision,
			"kill", models.ObjectKindScenery).
		Group("weapons.type, tunits.type").
		Order("kills DESC, weapons.type").
		Scan(&rows).Error; err != nil {
		logs.Sugar.Errorf("Failed to aggregate weapon matchups: %v", err)
		return nil, err
	}

	result := make([]*models.WeaponMatchup, 0, len(rows))
	for i := range rows {
		result = append(result, &rows[i])
	}

	return result, nil
}
