// Package web holds the dashboard: plain static files that talk to the GraphQL
// API over HTTP.
//
// It is embedded so the binary stays self-contained and the chart needs no extra
// volume or sidecar. Nothing here is server-rendered and nothing assumes it is
// served from the same origin as the API beyond the default, so moving the
// dashboard out later means serving this directory from somewhere else and
// setting window.OVERLORD_API_URL to point back at the API.
package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.css app.js
var files embed.FS

// FS returns the dashboard's static files.
func FS() fs.FS {
	return files
}
