package graph

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/MartinHell/overlord/controllers"
	"github.com/MartinHell/overlord/graph/generated"
	"github.com/MartinHell/overlord/models"
)

type Resolver struct{}

// ID is the resolver for the id field.
func (r *eventResolver) ID(ctx context.Context, obj *models.Event) (string, error) {
	return fmt.Sprintf("%v", obj.ID), nil
}

// PlayerID is the resolver for the playerID field.
func (r *killRecordResolver) PlayerID(ctx context.Context, obj *models.KillRecord) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

// ID is the resolver for the id field.
func (r *missionResolver) ID(ctx context.Context, obj *models.MissionSummary) (string, error) {
	return fmt.Sprintf("%v", obj.MissionID), nil
}

// PlayerID is the resolver for the playerID field.
func (r *playerResolver) PlayerID(ctx context.Context, obj *models.Player) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

// DeletedAt is the resolver for the deletedAt field.
func (r *playerResolver) DeletedAt(ctx context.Context, obj *models.Player) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

// PlayerID is the resolver for the playerID field.
func (r *playerActivityResolver) PlayerID(ctx context.Context, obj *models.PlayerActivity) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

// PlayerID is the resolver for the playerID field.
func (r *playerProfileResolver) PlayerID(ctx context.Context, obj *models.PlayerProfileView) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

// PlayerID is the resolver for the playerID field.
func (r *playerShotBreakdownResolver) PlayerID(ctx context.Context, obj *models.PlayerShotBreakdown) (string, error) {
	return fmt.Sprintf("%v", obj.PlayerID), nil
}

// Events is the resolver for the events field.
func (r *queryResolver) Events(ctx context.Context, first *int, after *string, eventType *string, coalition *string, missionID *string) (*models.EventConnection, error) {
	filterType := ""
	if eventType != nil {
		filterType = *eventType
	}

	filterCoalition := ""
	if coalition != nil {
		filterCoalition = *coalition
	}

	pageSize := 0
	if first != nil {
		pageSize = *first
	}

	cursor := ""
	if after != nil {
		cursor = *after
	}

	page, err := controllers.GetEventsPage(filterType, filterCoalition, pageSize, cursor, parseOptionalID(missionID))
	if err != nil {
		return nil, err
	}

	edges := make([]*models.EventEdge, 0, len(page.Events))
	for _, event := range page.Events {
		edges = append(edges, &models.EventEdge{
			Node:   event,
			Cursor: fmt.Sprintf("%d", event.ID),
		})
	}

	pageInfo := &models.PageInfo{HasNextPage: page.HasNextPage}
	if len(edges) > 0 {
		pageInfo.EndCursor = edges[len(edges)-1].Cursor
	}

	return &models.EventConnection{
		PageInfo: pageInfo,
		Edges:    edges,
	}, nil
}

// Missions is the resolver for the missions field.
func (r *queryResolver) Missions(ctx context.Context) ([]*models.MissionSummary, error) {
	return controllers.GetMissions()
}

// KillsByCoalition returns the kill tally for every coalition at once.
func (r *queryResolver) KillsByCoalition(ctx context.Context, missionID *string) ([]*models.CoalitionKills, error) {
	return controllers.GetKillsByCoalition(parseOptionalID(missionID))
}

// WeaponEffectiveness is the resolver for the weaponEffectiveness field.
func (r *queryResolver) WeaponEffectiveness(ctx context.Context, missionID *string) ([]*models.WeaponEffectiveness, error) {
	return controllers.GetWeaponEffectiveness(parseOptionalID(missionID))
}

// PlayerActivity is the resolver for the playerActivity field.
func (r *queryResolver) PlayerActivity(ctx context.Context, missionID *string) ([]*models.PlayerActivity, error) {
	return controllers.GetPlayerActivity(parseOptionalID(missionID))
}

// MissionTasks is the resolver for the missionTasks field.
func (r *queryResolver) MissionTasks(ctx context.Context, missionID *string) ([]*models.MissionTask, error) {
	return controllers.GetMissionTasks(parseOptionalID(missionID))
}

// Badges is the resolver for the badges field.
func (r *queryResolver) Badges(ctx context.Context, playerID string) ([]*models.Badge, error) {
	id := parseOptionalID(&playerID)
	if id == nil {
		return nil, nil
	}

	return controllers.GetBadges(*id)
}

// PlayerProfile is the resolver for the playerProfile field.
func (r *queryResolver) PlayerProfile(ctx context.Context, playerID string, unitType *string, missionID *string) (*models.PlayerProfileView, error) {
	// The argument name has to match the schema, since gqlgen regenerates this
	// signature; keep the parsed value under a different name.
	parsed, err := strconv.ParseUint(playerID, 10, 64)
	if err != nil {
		return nil, nil
	}

	narrow := ""
	if unitType != nil {
		narrow = *unitType
	}

	return controllers.GetPlayerProfile(uint(parsed), narrow, parseOptionalID(missionID))
}

// Records is the resolver for the records field.
func (r *queryResolver) Records(ctx context.Context, missionID *string) (*models.Records, error) {
	return controllers.GetRecords(parseOptionalID(missionID))
}

// Collateral is the resolver for the collateral field.
func (r *queryResolver) Collateral(ctx context.Context, playerID *string, missionID *string) (*models.Collateral, error) {
	// No player means the whole mission, which is the interesting number: DCS
	// reports most scenery damage with no initiator at all, so the sum across
	// every pilot is far short of the total.
	var id *uint
	if playerID != nil && *playerID != "" {
		parsed, err := strconv.ParseUint(*playerID, 10, 64)
		if err != nil {
			return nil, nil
		}
		v := uint(parsed)
		id = &v
	}

	return controllers.GetCollateral(id, parseOptionalID(missionID))
}

// LandingGrades is the resolver for the landingGrades field.
func (r *queryResolver) LandingGrades(ctx context.Context, first *int, missionID *string) ([]*models.LandingGrade, error) {
	limit := 0
	if first != nil {
		limit = *first
	}

	return controllers.GetLandingGrades(limit, parseOptionalID(missionID))
}

// UnitProfile is the resolver for the unitProfile field.
func (r *queryResolver) UnitProfile(ctx context.Context, typeArg string) (*models.UnitProfileView, error) {
	return controllers.GetUnitProfile(typeArg)
}

// WeaponProfile is the resolver for the weaponProfile field.
func (r *queryResolver) WeaponProfile(ctx context.Context, typeArg string) (*models.WeaponProfileView, error) {
	return controllers.GetWeaponProfile(typeArg)
}

// Event is the resolver for the event field.
func (r *queryResolver) Event(ctx context.Context, id string) (*models.Event, error) {
	return controllers.GetEvent(id), nil
}

// Players is the resolver for the players field.
func (r *queryResolver) Players(ctx context.Context) ([]*models.Player, error) {
	return controllers.GetPlayers()
}

// Player is the resolver for the player field.
func (r *queryResolver) Player(ctx context.Context, id string) (*models.Player, error) {
	return controllers.GetPlayerByID(id)
}

// Units is the resolver for the units field.
func (r *queryResolver) Units(ctx context.Context) ([]*models.Unit, error) {
	return controllers.GetUnits()
}

// Unit is the resolver for the unit field.
func (r *queryResolver) Unit(ctx context.Context, id string) (*models.Unit, error) {
	return controllers.GetUnitByID(id)
}

// Weapons is the resolver for the weapons field.
func (r *queryResolver) Weapons(ctx context.Context) ([]*models.Weapon, error) {
	return controllers.GetWeapons()
}

// Weapon is the resolver for the weapon field.
func (r *queryResolver) Weapon(ctx context.Context, id string) (*models.Weapon, error) {
	return controllers.GetWeaponByID(id)
}

// Healthcheck is the resolver for the healthcheck field.
func (r *queryResolver) Healthcheck(ctx context.Context) (string, error) {
	return "OK", nil
}

// ShotsBreakdown returns a breakdown of shots by unit and weapon type
func (r *queryResolver) ShotsBreakdown(ctx context.Context) ([]*models.UnitWeaponBreakdown, error) {
	return controllers.GetShotsBreakdown()
}

// ShotsByPlayers returns a breakdown of shots by all players
func (r *queryResolver) ShotsByPlayers(ctx context.Context) ([]*models.PlayerShotBreakdown, error) {
	return controllers.GetShotsByPlayers()
}

// ShotsByPlayer returns a breakdown of shots by a specific player
func (r *queryResolver) ShotsByPlayer(ctx context.Context, playerID string) (*models.PlayerShotBreakdown, error) {
	// The argument name has to match the schema, since gqlgen regenerates this
	// signature; keep the parsed value under a different name.
	var id uint
	if playerID != "" {
		parsed, _ := strconv.ParseUint(playerID, 10, 64)
		id = uint(parsed)
	}

	return controllers.GetShotsByPlayer(id)
}

// DisplayName is the resolver for the displayName field.
func (r *sceneryCountResolver) DisplayName(ctx context.Context, obj *models.SceneryCount) (string, error) {
	return models.SceneryName(obj.Type), nil
}

// TargetID is the resolver for the targetID field.
func (r *targetResolver) TargetID(ctx context.Context, obj *models.Target) (string, error) {
	return fmt.Sprintf("%v", obj.TargetID), nil
}

// UnitID is the resolver for the unitID field.
func (r *unitResolver) UnitID(ctx context.Context, obj *models.Unit) (string, error) {
	return fmt.Sprintf("%v", obj.UnitID), nil
}

// DisplayName is the resolver for the displayName field.
func (r *unitResolver) DisplayName(ctx context.Context, obj *models.Unit) (string, error) {
	profile, _ := models.UnitProfile(obj.Type)
	return profile.Name, nil
}

// DeletedAt is the resolver for the deletedAt field.
func (r *unitResolver) DeletedAt(ctx context.Context, obj *models.Unit) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

// UnitType is the resolver for the unitType field.
func (r *unitWeaponBreakdownResolver) UnitType(ctx context.Context, obj *models.UnitWeaponBreakdown) (string, error) {
	return obj.Unit, nil
}

// WeaponID is the resolver for the weaponID field.
func (r *weaponResolver) WeaponID(ctx context.Context, obj *models.Weapon) (string, error) {
	return fmt.Sprintf("%v", obj.WeaponID), nil
}

// DisplayName is the resolver for the displayName field.
func (r *weaponResolver) DisplayName(ctx context.Context, obj *models.Weapon) (string, error) {
	profile, _ := models.WeaponProfile(obj.Type)
	return profile.Name, nil
}

// DeletedAt is the resolver for the deletedAt field.
func (r *weaponResolver) DeletedAt(ctx context.Context, obj *models.Weapon) (*time.Time, error) {
	if obj.DeletedAt.Valid {
		return &obj.DeletedAt.Time, nil
	}
	return nil, nil
}

// Event returns generated.EventResolver implementation.
func (r *Resolver) Event() generated.EventResolver { return &eventResolver{r} }

// KillRecord returns generated.KillRecordResolver implementation.
func (r *Resolver) KillRecord() generated.KillRecordResolver { return &killRecordResolver{r} }

// Mission returns generated.MissionResolver implementation.
func (r *Resolver) Mission() generated.MissionResolver { return &missionResolver{r} }

// Player returns generated.PlayerResolver implementation.
func (r *Resolver) Player() generated.PlayerResolver { return &playerResolver{r} }

// PlayerActivity returns generated.PlayerActivityResolver implementation.
func (r *Resolver) PlayerActivity() generated.PlayerActivityResolver {
	return &playerActivityResolver{r}
}

// PlayerProfile returns generated.PlayerProfileResolver implementation.
func (r *Resolver) PlayerProfile() generated.PlayerProfileResolver { return &playerProfileResolver{r} }

// PlayerShotBreakdown returns generated.PlayerShotBreakdownResolver implementation.
func (r *Resolver) PlayerShotBreakdown() generated.PlayerShotBreakdownResolver {
	return &playerShotBreakdownResolver{r}
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// SceneryCount returns generated.SceneryCountResolver implementation.
func (r *Resolver) SceneryCount() generated.SceneryCountResolver { return &sceneryCountResolver{r} }

// Target returns generated.TargetResolver implementation.
func (r *Resolver) Target() generated.TargetResolver { return &targetResolver{r} }

// Unit returns generated.UnitResolver implementation.
func (r *Resolver) Unit() generated.UnitResolver { return &unitResolver{r} }

// UnitWeaponBreakdown returns generated.UnitWeaponBreakdownResolver implementation.
func (r *Resolver) UnitWeaponBreakdown() generated.UnitWeaponBreakdownResolver {
	return &unitWeaponBreakdownResolver{r}
}

// Weapon returns generated.WeaponResolver implementation.
func (r *Resolver) Weapon() generated.WeaponResolver { return &weaponResolver{r} }

type (
	eventResolver               struct{ *Resolver }
	killRecordResolver          struct{ *Resolver }
	missionResolver             struct{ *Resolver }
	playerResolver              struct{ *Resolver }
	playerActivityResolver      struct{ *Resolver }
	playerProfileResolver       struct{ *Resolver }
	playerShotBreakdownResolver struct{ *Resolver }
	queryResolver               struct{ *Resolver }
	sceneryCountResolver        struct{ *Resolver }
	targetResolver              struct{ *Resolver }
	unitResolver                struct{ *Resolver }
	unitWeaponBreakdownResolver struct{ *Resolver }
	weaponResolver              struct{ *Resolver }
)

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	type Resolver struct{}
*/
