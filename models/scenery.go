package models

import "strings"

// Scenery is the map furniture DCS reports as a target: trees, walls, houses,
// lamp posts. It is excluded from every hit and kill figure on purpose -- a
// blast catching a wood is not marksmanship -- but it is far too good to throw
// away, so it is counted separately and labelled for what it is.

// treeWords are the species and plant words that appear in DCS scenery
// identifiers, either as their own token (GREEN_ASH, HONEY_MESQUITE) or glued
// onto a place (AMERICANBEECH, ITALIANCYPRESS, LOMBARDYPOPLAR, NORWAYSPRUCE),
// which is why the match allows a suffix rather than only equality.
var treeWords = []string{
	"BEECH", "CYPRESS", "ASH", "MESQUITE", "POPLAR", "SPRUCE",
	"OAK", "PINE", "PALM", "WILLOW", "MAPLE", "BIRCH", "CEDAR", "ELM",
	"SHRUB", "TREE", "TREES",
}

// wreckWords mark the destroyed variant of a building: HOME1_CRASH,
// SKLAD_NEW_CRUSH. They have to be excluded before the suffix match, because
// CRASH ends in ASH and a burnt-out hangar would otherwise be filed as an ash
// tree. Twelve of the types on the test map end this way.
var wreckWords = map[string]bool{"CRASH": true, "CRUSH": true}

// SceneryName turns a scenery identifier into something readable.
//
// The generic prettifier leaves ITALIANCYPRESS and AMERICANBEECH glued
// together, which is fine for a type column and poor for a line of prose that
// is meant to raise a smile. Species names are split off the front word, and
// the variant noise DCS appends -- _NEW, _01 -- is dropped.
func SceneryName(sceneryType string) string {
	var out []string

	for _, token := range strings.FieldsFunc(strings.ToUpper(sceneryType), func(r rune) bool {
		return r == '_' || r == '-'
	}) {
		if token == "NEW" || isAllDigits(token) {
			continue
		}

		// AMERICANBEECH -> AMERICAN BEECH. Wreck words are skipped for the same
		// reason as in IsTree: CRASH would otherwise split into CR + ASH.
		if !wreckWords[token] {
			for _, word := range treeWords {
				if token != word && strings.HasSuffix(token, word) {
					out = append(out, title(strings.TrimSuffix(token, word)), title(word))
					token = ""
					break
				}
			}
		}

		if token != "" {
			out = append(out, title(token))
		}
	}

	if len(out) == 0 {
		return sceneryType
	}

	return strings.Join(out, " ")
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func title(s string) string {
	if s == "" {
		return s
	}
	return s[:1] + strings.ToLower(s[1:])
}

// IsTree reports whether a DCS scenery identifier names a plant.
//
// The grouping is read off the identifier, since DCS does not say what a
// scenery object is beyond its name. It is a guess, and the UI says so.
func IsTree(sceneryType string) bool {
	for _, token := range strings.FieldsFunc(strings.ToUpper(sceneryType), func(r rune) bool {
		return r == '_' || r == '-'
	}) {
		if wreckWords[token] {
			continue
		}
		for _, word := range treeWords {
			if token == word || strings.HasSuffix(token, word) {
				return true
			}
		}
	}

	return false
}
