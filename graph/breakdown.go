package graph

// Aggregation helpers used by the query resolvers. They live outside
// resolver.go because gqlgen owns and rewrites that file on every generate.

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/MartinHell/overlord/models"
)

func convertStringToUint(s string) uint {
	value, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// The only caller builds these strings from uint keys, so a parse
		// failure means a bug rather than bad user input. Zero is a safe
		// sentinel; panicking here would take down the request.
		return 0
	}
	return uint(value)
}

// Helper functions
// generateBreakdownUnits generates a breakdown of shots by unit and weapon type
func generateBreakdownUnits(events []*models.Event) map[string]map[string]int {
	breakdown := make(map[string]map[string]int)

	for _, event := range events {
		// Check if unit type is present in the event
		if event.Initiator.Type == "" {
			continue // Skip events without unit type
		}

		unitType := event.Initiator.Type
		weaponType := event.Weapon.Type

		if breakdown[unitType] == nil {
			breakdown[unitType] = make(map[string]int)
		}
		breakdown[unitType][weaponType]++
	}

	return breakdown
}

// generateUnitWeaponBreakdown generates a slice of UnitWeaponBreakdown structs
func generateUnitWeaponBreakdown(breakdown map[string]map[string]int) []*models.UnitWeaponBreakdown {
	var result []*models.UnitWeaponBreakdown

	for unitType, weapons := range breakdown {
		unit := &models.UnitWeaponBreakdown{
			Unit:    unitType,
			Weapons: []*models.WeaponShotBreakdown{},
		}
		for weaponType, count := range weapons {
			unit.Weapons = append(unit.Weapons, &models.WeaponShotBreakdown{
				WeaponType: weaponType,
				Count:      count,
			})
		}
		result = append(result, unit)
	}

	return result
}

// generateBreakdown generates a breakdown of shots by player
func generateBreakdown(events []*models.Event) (map[string]map[string]map[string]int, map[string]string) {
	breakdown := make(map[string]map[string]map[string]int)
	playerNames := make(map[string]string)

	for _, event := range events {
		// Check if player is present in the event
		if event.Player.PlayerName == nil || event.PlayerID == nil {
			continue // Skip events without player information
		}

		playerID := fmt.Sprintf("%d", *event.PlayerID)
		playerNames[playerID] = *event.Player.PlayerName // Store player name for each player ID

		// Check if unit type and weapon type are present in the event
		if event.Initiator.Type == "" || event.Weapon.Type == "" {
			continue // Skip events without unit type or weapon type
		}

		unitType := event.Initiator.Type
		weaponType := event.Weapon.Type

		if breakdown[playerID] == nil {
			breakdown[playerID] = make(map[string]map[string]int)
		}
		if breakdown[playerID][unitType] == nil {
			breakdown[playerID][unitType] = make(map[string]int)
		}
		breakdown[playerID][unitType][weaponType]++
	}

	return breakdown, playerNames

}

// generatePlayerShotBreakdown generates a slice of PlayerShotBreakdown structs
func generatePlayerShotBreakdown(breakdown map[string]map[string]map[string]int, playerNames map[string]string) []*models.PlayerShotBreakdown {
	var result []*models.PlayerShotBreakdown

	for playerID, units := range breakdown {
		player := &models.PlayerShotBreakdown{
			PlayerID:   convertStringToUint(playerID),
			PlayerName: playerNames[playerID], // Retrieve the player name
			Units:      []*models.UnitShotBreakdown{},
		}
		for unitType, weapons := range units {
			unit := &models.UnitShotBreakdown{
				UnitType: unitType,
				Weapons:  []*models.WeaponShotBreakdown{},
			}
			for weaponType, count := range weapons {
				unit.Weapons = append(unit.Weapons, &models.WeaponShotBreakdown{
					WeaponType: weaponType,
					Count:      count,
				})
			}
			player.Units = append(player.Units, unit)
		}
		result = append(result, player)
	}

	// Sort result alphabetically based on player names
	sort.Slice(result, func(i, j int) bool {
		return result[i].PlayerName < result[j].PlayerName
	})

	return result
}

// GeneratePlayerShotBreakdowns generates a breakdown of shots by player
// and returns a slice of PlayerShotBreakdown structs
// Each PlayerShotBreakdown struct contains the player ID, player name, and a slice of UnitShotBreakdown structs
// Each UnitShotBreakdown struct contains the unit type and a slice of WeaponShotBreakdown structs
// Each WeaponShotBreakdown struct contains the weapon type and the number of shots fired
func GeneratePlayerShotBreakdowns(events []*models.Event) ([]*models.PlayerShotBreakdown, error) {
	breakdown, playerNames := generateBreakdown(events)

	result := generatePlayerShotBreakdown(breakdown, playerNames)

	return result, nil
}
