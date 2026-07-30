package graph

// Helpers live outside resolver.go on purpose: gqlgen rewrites that file on
// every schema change and moves anything that is not a resolver out into a
// commented graveyard at the bottom, which deleted parseOptionalID the first
// time the schema was regenerated after it was added.

import "strconv"

// parseOptionalID turns an optional GraphQL ID into a *uint, treating absent,
// empty and unparseable all as "not narrowed" rather than as errors.
func parseOptionalID(s *string) *uint {
	if s == nil || *s == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(*s, 10, 64)
	if err != nil {
		return nil
	}
	v := uint(parsed)
	return &v
}
