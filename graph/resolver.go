package graph

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/MartinHell/overlord/controllers"
	"github.com/MartinHell/overlord/graph/generated"
	"github.com/MartinHell/overlord/models"
)

func convertStringToUint(s string) uint {
	// Convert string to uint
	value, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// Handle error if the string cannot be converted
		panic(err)
	}
	return uint(value)
}

type Resolver struct {
}

func (r *Resolver) Query() generated.QueryResolver {
	return &queryResolver{r}
}

func (r *Resolver) Event() generated.EventResolver {
	return &eventResolver{r}
}

func (r *Resolver) Player() generated.PlayerResolver {
	return &playerResolver{r}
}

func (r *Resolver) Unit() generated.UnitResolver {
	return &unitResolver{r}
}

func (r *Resolver) Weapon() generated.WeaponResolver {
	return &weaponResolver{r}
}

func (r *Resolver) PlayerShotBreakdown() generated.PlayerShotBreakdownResolver {
	return &playerShotBreakdownResolver{r}
}

func (r *Resolver) UnitWeaponBreakdown() generated.UnitWeaponBreakdownResolver {
	return &unitWeaponBreakdownResolver{r}
}

type queryResolver struct{ *Resolver }

func (r *queryResolver) Healthcheck(ctx context.Context) (string, error) {
	return "OK", nil
}

func (r *queryResolver) Events(ctx context.Context, first *int, after *string, eventType *string) (*models.EventConnection, error) {
	events := []*models.Event{}

	if eventType != nil && *eventType != "" {
		events = controllers.GetEventsByType(*eventType)
	} else {
		events = controllers.GetEvents()
	}

	start := 0
	if after != nil {
		for i, event := range events {
			if fmt.Sprintf("%d", event.ID) == *after {
				start = i
				break
			}
		}
	}

	// Get the slice of events
	end := start + *first
	if end > len(events) {
		end = len(events)
	}
	slicedEvents := events[start:end]

	// Create EventEdges
	var edges []*models.EventEdge
	for _, event := range slicedEvents {
		edges = append(edges, &models.EventEdge{
			Node:   event,
			Cursor: fmt.Sprintf("%d", event.ID),
		})
	}

	// Create PageInfo
	pageInfo := &models.PageInfo{
		EndCursor:   fmt.Sprintf("%d", events[end-1].ID),
		HasNextPage: end < len(events),
	}

	return &models.EventConnection{
		PageInfo: pageInfo,
		Edges:    edges,
	}, nil
}

// TODO: Refactor this to return the breakdown of shots on type of unit:
// Somethign like this:
// - UnitType
//   - WeaponType
//     Count
//   - WeaponType
//     Count
func (r *queryResolver) ShotsBreakdown(ctx context.Context) ([]*models.UnitWeaponBreakdown, error) {
	events := controllers.GetEventsByType("shot")
	breakdown := make(map[string]map[string]int)

	for _, event := range events {
		unitType := event.Initiator.Type
		weaponType := event.Weapon.Type

		if breakdown[unitType] == nil {
			breakdown[unitType] = make(map[string]int)
		}
		breakdown[unitType][weaponType]++
	}

	var result []*models.UnitWeaponBreakdown
	for unitType, weapons := range breakdown {
		for weaponType, count := range weapons {
			result = append(result, &models.UnitWeaponBreakdown{
				Unit:   unitType,
				Weapon: weaponType,
				Count:  count,
			})
		}
	}

	return result, nil
}

func (r *queryResolver) ShotsByPlayers(ctx context.Context) ([]*models.PlayerShotBreakdown, error) {
	events := controllers.GetEventsByType("shot")
	if events == nil {
		return nil, nil // or handle the nil events case as needed
	}

	return GeneratePlayerShotBreakdowns(events)
}

func (r *queryResolver) ShotsByPlayer(ctx context.Context, pID string) (*models.PlayerShotBreakdown, error) {
	var playerID uint
	if pID != "" {
		tmpID, _ := strconv.ParseUint(pID, 10, 64)
		playerID = uint(tmpID)
	}

	var player models.Player

	player.GetPlayerByPlayerID(playerID)

	events := controllers.GetEventsByTypeAndPlayer("shot", playerID)
	if events == nil {
		return nil, nil // or handle the nil events case as needed
	}

	var result []*models.PlayerShotBreakdown

	result, err := GeneratePlayerShotBreakdowns(events)
	if err != nil {
		return nil, err
	}

	return result[0], nil
}

func (r *queryResolver) Event(ctx context.Context, id string) (*models.Event, error) {
	return controllers.GetEvent(id), nil
}

func (r *queryResolver) Player(ctx context.Context, id string) (*models.Player, error) {
	return &models.Player{}, nil
}

func (r *queryResolver) Players(ctx context.Context) ([]*models.Player, error) {
	return []*models.Player{}, nil
}

func (r *queryResolver) PlayerEvents(ctx context.Context, id string) ([]*models.Event, error) {
	return []*models.Event{}, nil
}

func (r *queryResolver) Units(ctx context.Context) ([]*models.Unit, error) {
	return []*models.Unit{}, nil
}

func (r *queryResolver) Unit(ctx context.Context, id string) (*models.Unit, error) {
	return &models.Unit{}, nil
}

func (r *queryResolver) Weapons(ctx context.Context) ([]*models.Weapon, error) {
	return []*models.Weapon{}, nil
}

func (r *queryResolver) Weapon(ctx context.Context, id string) (*models.Weapon, error) {
	return &models.Weapon{}, nil
}

type eventResolver struct{ *Resolver }

func (r *eventResolver) ID(ctx context.Context, obj *models.Event) (string, error) {
	return fmt.Sprintf("%v", obj.ID), nil
}

func (r *eventResolver) player(ctx context.Context, obj *models.Event) (*models.Player, error) {
	return &models.Player{}, nil
}

func (r *eventResolver) initiator(ctx context.Context, obj *models.Event) (*models.Unit, error) {
	return &models.Unit{}, nil
}

func (r *eventResolver) target(ctx context.Context, obj *models.Event) (*models.Unit, error) {
	return &models.Unit{}, nil
}

func (r *eventResolver) weapon(ctx context.Context, obj *models.Event) (*models.Weapon, error) {
	return &models.Weapon{}, nil
}

type playerResolver struct{ *Resolver }

func (r *playerResolver) DeletedAt(ctx context.Context, obj *models.Player) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

func (r *playerResolver) PlayerID(ctx context.Context, obj *models.Player) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

type unitResolver struct{ *Resolver }

func (r *unitResolver) DeletedAt(ctx context.Context, obj *models.Unit) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

func (r *unitResolver) UnitID(ctx context.Context, obj *models.Unit) (string, error) {
	return fmt.Sprintf("%v", obj.UnitID), nil
}

type weaponResolver struct{ *Resolver }

func (r *weaponResolver) DeletedAt(ctx context.Context, obj *models.Weapon) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

func (r *weaponResolver) WeaponID(ctx context.Context, obj *models.Weapon) (string, error) {
	return fmt.Sprintf("%v", obj.WeaponID), nil
}

type playerShotBreakdownResolver struct{ *Resolver }

func (r *playerShotBreakdownResolver) PlayerID(ctx context.Context, obj *models.PlayerShotBreakdown) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

func (r *playerShotBreakdownResolver) PlayerName(ctx context.Context, obj *models.PlayerShotBreakdown) (string, error) {
	return obj.PlayerName, nil
}

func (r *playerShotBreakdownResolver) Units(ctx context.Context, obj *models.PlayerShotBreakdown) ([]*models.UnitShotBreakdown, error) {
	return obj.Units, nil
}

type unitShotBreakdownResolver struct{ *Resolver }

func (r *unitShotBreakdownResolver) UnitType(ctx context.Context, obj *models.UnitShotBreakdown) (string, error) {
	return obj.UnitType, nil
}

func (r *unitShotBreakdownResolver) Weapons(ctx context.Context, obj *models.UnitShotBreakdown) ([]*models.WeaponShotBreakdown, error) {
	return obj.Weapons, nil
}

type weaponShotBreakdownResolver struct{ *Resolver }

func (r *weaponShotBreakdownResolver) WeaponType(ctx context.Context, obj *models.WeaponShotBreakdown) (string, error) {
	return obj.WeaponType, nil
}

func (r *weaponShotBreakdownResolver) Count(ctx context.Context, obj *models.WeaponShotBreakdown) (int, error) {
	return obj.Count, nil
}

type unitWeaponBreakdownResolver struct{ *Resolver }

func (r *unitWeaponBreakdownResolver) Count(ctx context.Context, obj *models.UnitWeaponBreakdown) (int, error) {
	return obj.Count, nil
}

func (r *unitWeaponBreakdownResolver) UnitType(ctx context.Context, obj *models.UnitWeaponBreakdown) (string, error) {
	return obj.Unit, nil
}

func (r *unitWeaponBreakdownResolver) WeaponType(ctx context.Context, obj *models.UnitWeaponBreakdown) (string, error) {
	return obj.Weapon, nil
}

// Helper functions

func generateBreakdown(events []*models.Event) (map[string]map[string]map[string]int, map[string]string) {
	breakdown := make(map[string]map[string]map[string]int)
	playerNames := make(map[string]string)

	for _, event := range events {
		// Check if player is present in the event
		if event.Player.PlayerName == nil {
			continue // Skip events without player information
		}

		playerID := fmt.Sprintf("%d", *event.PlayerID)
		playerNames[playerID] = *event.Player.PlayerName // Store player name for each player ID

		// Check if unit type and weapon type are present in the event
		if &event.Initiator == nil || event.Initiator.Type == "" || &event.Weapon == nil || event.Weapon.Type == "" {
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
