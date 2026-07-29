package controllers

import (
	"testing"

	"github.com/MartinHell/overlord/models"
)

// tallyKills mirrors the aggregation in GetKillsByCoalition without the
// database round trip, so the counting rules can be tested directly.
func tallyKills(events []*models.Event) []*models.CoalitionKills {
	order := []string{}
	tally := map[string]*models.CoalitionKills{}

	for _, event := range events {
		coalition := event.Coalition
		if coalition == "" {
			coalition = models.CoalitionUnknown
		}

		if tally[coalition] == nil {
			tally[coalition] = &models.CoalitionKills{Coalition: coalition}
			order = append(order, coalition)
		}

		tally[coalition].Kills++
		if coalition != models.CoalitionUnknown && event.TargetCoalition == coalition {
			tally[coalition].Teamkills++
		}
	}

	result := make([]*models.CoalitionKills, 0, len(order))
	for _, coalition := range order {
		result = append(result, tally[coalition])
	}
	return result
}

func TestKillTallySeparatesCoalitions(t *testing.T) {
	events := []*models.Event{
		{Coalition: models.CoalitionBlue, TargetCoalition: models.CoalitionRed},
		{Coalition: models.CoalitionBlue, TargetCoalition: models.CoalitionRed},
		{Coalition: models.CoalitionRed, TargetCoalition: models.CoalitionBlue},
	}

	got := tallyKills(events)

	if len(got) != 2 {
		t.Fatalf("expected two coalitions, got %d", len(got))
	}
	if got[0].Coalition != models.CoalitionBlue || got[0].Kills != 2 {
		t.Errorf("expected blue with 2 kills, got %+v", got[0])
	}
	if got[1].Coalition != models.CoalitionRed || got[1].Kills != 1 {
		t.Errorf("expected red with 1 kill, got %+v", got[1])
	}
}

func TestKillTallyCountsTeamkills(t *testing.T) {
	events := []*models.Event{
		{Coalition: models.CoalitionBlue, TargetCoalition: models.CoalitionBlue},
		{Coalition: models.CoalitionBlue, TargetCoalition: models.CoalitionRed},
	}

	got := tallyKills(events)

	if got[0].Kills != 2 {
		t.Fatalf("expected 2 kills, got %d", got[0].Kills)
	}
	if got[0].Teamkills != 1 {
		t.Fatalf("expected 1 teamkill, got %d", got[0].Teamkills)
	}
}

func TestKillTallyDoesNotTreatUnknownAsTeamkill(t *testing.T) {
	// Both of these have an unknown coalition on each side. They compare equal
	// but tell us nothing, so they must not be reported as teamkills.
	events := []*models.Event{
		{Coalition: models.CoalitionUnknown, TargetCoalition: models.CoalitionUnknown},
		// Historical events recorded before coalition tracking existed.
		{Coalition: "", TargetCoalition: ""},
	}

	got := tallyKills(events)

	if len(got) != 1 || got[0].Coalition != models.CoalitionUnknown {
		t.Fatalf("expected a single unknown bucket, got %+v", got)
	}
	if got[0].Kills != 2 {
		t.Errorf("expected 2 kills, got %d", got[0].Kills)
	}
	if got[0].Teamkills != 0 {
		t.Errorf("expected 0 teamkills for unknown coalitions, got %d", got[0].Teamkills)
	}
}
