//go:build tools

// Package tools pins the build-time dependencies that are not imported by the
// application itself. Without this, `go mod tidy` drops gqlgen's own
// dependencies and `go run github.com/99designs/gqlgen generate` stops working.
package tools

import (
	_ "github.com/99designs/gqlgen"
)
