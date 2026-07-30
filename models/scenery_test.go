package models

import "testing"

// Every identifier here was taken from the events table of a live mission, so
// this is a record of what DCS actually emits rather than what it might.
func TestIsTree(t *testing.T) {
	trees := []string{
		"EUROPEAN_BEECH", "ITALIANCYPRESS", "GREEN_ASH", "AMERICANBEECH",
		"HONEY_MESQUITE", "LOMBARDYPOPLAR", "NORWAYSPRUCE", "SHRUB",
		"TREES_9_NEW", "TREES_1_NEW",
	}
	for _, s := range trees {
		if !IsTree(s) {
			t.Errorf("IsTree(%q) = false, want true", s)
		}
	}

	// CRASH and CRUSH end in ASH and USH; without the wreck-word guard the
	// first four of these get counted as ash trees.
	notTrees := []string{
		"SMALL_CRASH", "HOME1_CRASH", "GARAGE_A_CRASH", "ANGAR_A_CRASH",
		"SKLAD_NEW_CRUSH", "BAK_NEW_CRUSH", "HOME1UG_CRUSH",
		"CONCRETE_WALL_01", "BLK_LIGHT_POLE", "POWERTRANSPOLE_ROAD_01",
		"HOME1UG_A", "DOMIK1B_NEW", "KOTELNAYA_A_NEW", "MAGAZIN_NEW",
		"BARREL", "WOOD_BOX_01", "BATUMI_FONAR", "MOST(ROAD)BIG",
	}
	for _, s := range notTrees {
		if IsTree(s) {
			t.Errorf("IsTree(%q) = true, want false", s)
		}
	}
}

func TestSceneryName(t *testing.T) {
	cases := map[string]string{
		"EUROPEAN_BEECH":   "European Beech",
		"ITALIANCYPRESS":   "Italian Cypress",
		"AMERICANBEECH":    "American Beech",
		"LOMBARDYPOPLAR":   "Lombardy Poplar",
		"NORWAYSPRUCE":     "Norway Spruce",
		"HONEY_MESQUITE":   "Honey Mesquite",
		"GREEN_ASH":        "Green Ash",
		"SHRUB":            "Shrub",
		"CONCRETE_WALL_01": "Concrete Wall",
		"TREES_9_NEW":      "Trees",
		// Must not split into "Small Cr Ash".
		"SMALL_CRASH": "Small Crash",
	}

	for in, want := range cases {
		if got := SceneryName(in); got != want {
			t.Errorf("SceneryName(%q) = %q, want %q", in, got, want)
		}
	}
}
