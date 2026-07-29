package models

import "testing"

func TestUnitProfileCurated(t *testing.T) {
	p, known := UnitProfile("F-16C_50")

	if !known {
		t.Fatal("F-16C_50 should be curated")
	}
	if p.Name != "F-16C Block 50" {
		t.Errorf("expected a readable name, got %q", p.Name)
	}
	if p.Nickname != "Viper" {
		t.Errorf("expected the Viper nickname, got %q", p.Nickname)
	}
}

func TestWeaponProfileCurated(t *testing.T) {
	p, known := WeaponProfile("AIM_54C_Mk47")

	if !known {
		t.Fatal("AIM_54C_Mk47 should be curated")
	}
	if p.Name != "AIM-54C Phoenix" {
		t.Errorf("expected AIM-54C Phoenix, got %q", p.Name)
	}
}

// An unlisted type must still read sensibly, and must report itself as
// uncurated so the UI can say so rather than implying the data is verified.
func TestUnknownTypesArePrettifiedNotInvented(t *testing.T) {
	cases := map[string]string{
		"weapons.shells.M61_20_HE": "M61 20 HE",
		"SOME_NEW_JET":             "SOME NEW JET",
		"Tor 9A331":                "Tor 9A331",
	}

	for in, want := range cases {
		got := prettify(in)
		if got != want {
			t.Errorf("prettify(%q) = %q, want %q", in, got, want)
		}
	}

	if _, known := WeaponProfile("TOTALLY_MADE_UP"); known {
		t.Error("an unlisted weapon must not report itself as curated")
	}
}

// Curated entries carry no performance figures on purpose; this guards against
// someone adding unsourced specs to the Blurb later.
func TestProfilesCarryNoPerformanceClaims(t *testing.T) {
	banned := []string{"mach ", "knots", " kt ", "km/h", " mph", "ceiling", "nm range", "lbf", "thrust"}

	check := func(kind string, table map[string]Profile) {
		for key, p := range table {
			lower := toLower(p.Blurb + " " + p.Role)
			for _, b := range banned {
				if contains(lower, b) {
					t.Errorf("%s %q mentions %q; performance figures need a source, not recall", kind, key, b)
				}
			}
			if p.Name == "" {
				t.Errorf("%s %q has no name", kind, key)
			}
		}
	}

	check("unit", unitProfiles)
	check("weapon", weaponProfiles)
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
