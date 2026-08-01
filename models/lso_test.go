package models

import "testing"

// Strings taken verbatim from the recorded database, so the parser is tested
// against what DCS actually writes rather than what the manual says it does.
func TestParseLSO(t *testing.T) {
	cases := []struct {
		in     string
		grade  string
		score  float64
		wire   int
		devs   int
		graded bool
	}{
		{"LSO: GRADE:_OK_ : WIRE# 4", "_OK_", 4.0, 4, 0, true},
		{"LSO: GRADE:C : LNFIW  WIRE# 2", "C", 1.0, 2, 1, true},
		{"LSO: GRADE:--- : (EGTL)  WIRE# 4", "---", 2.0, 4, 1, true},
		{"LSO: GRADE:C : _TMRDIC_  (EGTL)  LNFIW  WIRE# 1", "C", 1.0, 1, 3, true},
		{"LSO: GRADE:--- : LOAR  (EGTL)  WIRE# 2", "---", 2.0, 2, 2, true},
		// A bolter catches no wire, which is not the same as wire zero being
		// missing from the string.
		{"LSO: GRADE:B :", "B", 2.0, 0, 0, true},
	}

	for _, c := range cases {
		got, ok := ParseLSO(c.in)
		if !ok {
			t.Errorf("ParseLSO(%q) reported no grade", c.in)
			continue
		}
		if got.Grade != c.grade {
			t.Errorf("ParseLSO(%q) grade = %q, want %q", c.in, got.Grade, c.grade)
		}
		if got.Score != c.score {
			t.Errorf("ParseLSO(%q) score = %v, want %v", c.in, got.Score, c.score)
		}
		if got.Graded != c.graded {
			t.Errorf("ParseLSO(%q) graded = %v, want %v", c.in, got.Graded, c.graded)
		}
		if got.Wire != c.wire {
			t.Errorf("ParseLSO(%q) wire = %d, want %d", c.in, got.Wire, c.wire)
		}
		if len(got.Deviations) != c.devs {
			t.Errorf("ParseLSO(%q) deviations = %v, want %d of them", c.in, got.Deviations, c.devs)
		}
	}
}

// A landing DCS graded some other way, or not at all, must not come back as a
// zero-scored pass -- that would drag every average down with phantom traps.
func TestParseLSOIgnoresNonCarrierLandings(t *testing.T) {
	for _, in := range []string{"", "landed", "Airfield landing at Batumi"} {
		if _, ok := ParseLSO(in); ok {
			t.Errorf("ParseLSO(%q) claimed to be a graded pass", in)
		}
	}
}

// An unknown grade keeps its name and is excluded from scoring rather than
// counted as a zero.
func TestParseLSOUnknownGrade(t *testing.T) {
	got, ok := ParseLSO("LSO: GRADE:?? : WIRE# 3")
	if !ok {
		t.Fatal("expected a parse")
	}
	if got.Grade != "??" || got.Graded {
		t.Errorf("got grade %q graded=%v, want %q ungraded", got.Grade, got.Graded, "??")
	}
	if got.Wire != 3 {
		t.Errorf("wire = %d, want 3", got.Wire)
	}
}
