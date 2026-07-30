package graph

import (
	"os"
	"strings"
	"testing"
)

// gqlgen scaffolds new resolvers as panic("not implemented"). That compiles, so
// nothing catches a forgotten one until a query hits it at runtime and degrades
// to "internal system error" -- which is exactly what happened when
// PlayerActivity.playerID was added and missed.
func TestNoUnimplementedResolvers(t *testing.T) {
	for _, path := range []string{"resolver.go", "scalar.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, `panic("not implemented`) {
				t.Errorf("%s:%d has an unimplemented resolver: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
