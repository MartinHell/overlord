package models

import (
	"regexp"
	"strconv"
	"strings"
)

// Parsing the LSO's grade.
//
// DCS reports a carrier recovery as one string in the Landing Signal Officer's
// own shorthand:
//
//	LSO: GRADE:_OK_ : WIRE# 4
//	LSO: GRADE:C : LNFIW  WIRE# 2
//	LSO: GRADE:--- : (EGTL)  LNFIW  WIRE# 3
//
// Displayed raw it is unreadable to anyone who has not flown the pattern, and
// unusable for anything but reading. Split into a grade, a wire and a list of
// deviations it becomes a score you can average, a distribution you can chart,
// and a pass you can compare with the one before it.
//
// Parsed here rather than in the browser because it is a property of the
// recording, not of one page: the chart, the table and any future export should
// all agree about what a pass was worth.

var (
	lsoGrade = regexp.MustCompile(`GRADE:\s*([^\s:]+)`)
	lsoWire  = regexp.MustCompile(`WIRE#\s*(\d+)`)
)

// lsoScores are the Navy's grades and what each is worth. A perfect pass is
// four points; a cut pass -- one the LSO judged unsafe -- is one.
//
// Values follow the standard USN scale so an average here means what it means
// in a ready room. Anything unrecognised scores nothing and is left named.
var lsoScores = map[string]float64{
	"_OK_": 4.0, // perfect
	"OK":   3.0, // no deviations worth mentioning
	"(OK)": 2.5, // fair
	"---":  2.0, // no grade
	"B":    2.0, // bolter -- missed the wires, not a judgement of the pass
	"C":    1.0, // cut: unsafe
	"WO":   0.0, // wave off
}

// lsoWords expands the LSO's abbreviations. Only position and error codes are
// listed; the parser leaves anything else as it found it rather than guessing.
var lsoWords = map[string]string{
	"LU": "lined up left", "LUR": "lined up right", "LUL": "lined up left",
	"OS": "overshoot", "US": "undershoot",
	"H": "high", "LO": "low", "F": "fast", "SLO": "slow",
	"DL": "drifted left", "DR": "drifted right",
	"P": "power", "N": "nose", "LL": "landed left", "LR": "landed right",
	"EG": "eased gun", "TL": "too long", "AR": "at the ramp", "IC": "in close",
	"IM": "in the middle", "X": "at the start", "IW": "in the wires",
	"LNF": "not enough line-up correction", "TMRD": "too much rate of descent",
	"NSU": "not set up", "WOP": "wave-off pattern", "LOAR": "low at the ramp",
	"LOIC": "low in close", "LOIM": "low in the middle",
}

// LSOPass is one carrier recovery, read out of the LSO's shorthand.
type LSOPass struct {
	// Grade is the token itself: _OK_, OK, (OK), ---, B, C, WO.
	Grade string
	// Score is what that grade is worth on the four-point scale, and Graded
	// says whether the token was one we know. An unrecognised grade scores
	// zero, which is not the same as having earned zero.
	Score  float64
	Graded bool
	// Wire caught, 1 to 4. Zero when the pass caught none, which is a bolter.
	Wire int
	// Deviations are the LSO's remarks, expanded where the abbreviation is
	// known. Empty for a clean pass.
	Deviations []string
}

// ParseLSO reads a DCS landing-quality string. A string that carries no grade
// at all returns ok false: it is a landing DCS graded some other way, or none.
func ParseLSO(comment string) (LSOPass, bool) {
	if comment == "" {
		return LSOPass{}, false
	}

	// Indices, not the matched text: the grade sits partway into the string, so
	// slicing by the match's length rather than its end takes a bite out of
	// "LSO: GRADE:" and leaves it in the remarks.
	m := lsoGrade.FindStringSubmatchIndex(comment)
	if m == nil {
		return LSOPass{}, false
	}

	pass := LSOPass{Grade: comment[m[2]:m[3]]}
	if score, known := lsoScores[pass.Grade]; known {
		pass.Score = score
		pass.Graded = true
	}

	if w := lsoWire.FindStringSubmatch(comment); w != nil {
		pass.Wire, _ = strconv.Atoi(w[1])
	}

	// What is left between the grade and the wire is the LSO's remarks. The
	// case and brackets carry severity -- lower case is minor, upper case
	// major, underscores gross -- which is preserved by expanding the letters
	// and leaving the wrapping alone.
	rest := comment[m[1]:]
	rest = lsoWire.ReplaceAllString(rest, "")
	rest = strings.TrimLeft(rest, ": ")

	for _, token := range strings.Fields(rest) {
		if word := expandLSO(token); word != "" {
			pass.Deviations = append(pass.Deviations, word)
		}
	}

	return pass, true
}

// expandLSO turns one remark into words, keeping the severity its wrapping
// carries. Unknown codes come back as they went in: a remark nobody can read
// is still better than one silently dropped.
func expandLSO(token string) string {
	trimmed := strings.Trim(token, "_()")
	if trimmed == "" {
		return ""
	}

	severity := ""
	switch {
	case strings.HasPrefix(token, "_"):
		severity = "gross "
	case strings.HasPrefix(token, "("):
		severity = "a little "
	}

	return severity + decodeLSO(strings.ToUpper(trimmed))
}

// decodeLSO reads one remark, which the LSO writes as run-together codes:
// LNFIW is LNF then IW, "not enough line-up correction in the wires"; TMRDIC
// is TMRD then IC. Longest match first, so LOAR is read as itself rather than
// as LO followed by AR.
//
// A code nobody recognises comes back as it went in. A remark that cannot be
// read is still better than one silently dropped.
func decodeLSO(code string) string {
	if word, known := lsoWords[code]; known {
		return word
	}

	var out []string
	for i := 0; i < len(code); {
		matched := ""
		for size := len(code) - i; size > 0; size-- {
			if word, known := lsoWords[code[i:i+size]]; known {
				matched = word
				i += size
				break
			}
		}
		if matched == "" {
			// Nothing here is a code. Keep the rest verbatim and stop.
			out = append(out, code[i:])
			break
		}
		out = append(out, matched)
	}

	return strings.Join(out, " ")
}
