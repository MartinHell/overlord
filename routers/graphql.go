package routers

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/MartinHell/overlord/graph"
	"github.com/MartinHell/overlord/graph/generated"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/web"
	"github.com/vektah/gqlparser/v2/ast"
)

// queryComplexityLimit caps how expensive one GraphQL query may be.
//
// Without it a caller can nest connections until the resolvers do unbounded
// work -- the schema has no depth limit of its own, and every field is free to
// ask. The number is generous next to what the dashboard sends: its heaviest
// page load scores in the low hundreds, so this leaves room to grow while still
// refusing anything pathological.
const queryComplexityLimit = 2000

// maxRequestBytes bounds a request body. A GraphQL query is a few kilobytes at
// most; anything larger is either a mistake or an attempt to make the parser
// work for its living.
const maxRequestBytes = 1 << 20 // 1 MiB

func GraphQLHandler() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}
	development := environment != "production"

	// Host to bind. Empty means every interface, which is what this has always
	// done and what a container needs. Set HOST=127.0.0.1 to keep the API on
	// the machine it runs on, which is the right setting for a dashboard beside
	// a DCS server until there is authentication in front of it (see #36).
	host := os.Getenv("HOST")

	// Built explicitly rather than with NewDefaultServer, which mounts things
	// this API does not have and should not offer: a websocket transport with
	// no subscriptions in the schema, multipart form handling with no uploads,
	// and introspection unconditionally.
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.FixedComplexityLimit(queryComplexityLimit))

	// Introspection is what lets a client discover the whole schema. The
	// playground needs it; nothing else here does, so the two are gated
	// together.
	if development {
		srv.Use(extension.Introspection{})
	}

	mux := http.NewServeMux()
	mux.Handle("/query", http.MaxBytesHandler(srv, maxRequestBytes))

	// Health endpoint for container probes. Deliberately separate from anything
	// that renders, so it stays valid whatever else is or is not mounted.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Static files only; see the web package for why this is not server-rendered.
	//
	// Embedded files carry a zero modification time, so http.FileServer cannot
	// send Last-Modified and browsers fall back to heuristic caching. That
	// silently serves a stale dashboard after an upgrade, which is indistinguishable
	// from the code being wrong. Require revalidation instead: these files are a
	// few kilobytes and the dashboard polls anyway.
	dashboard := http.FileServer(http.FS(web.FS()))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		dashboard.ServeHTTP(w, r)
	}))

	// A pilot's record is a page at /player/<id> and a mission recap one at
	// /mission/<id>, so both have to be served on a refresh, a bookmark or a
	// pasted link and not only when the dashboard happens to navigate there.
	// The id is read from the path by the client; the server's job is just to
	// hand over the document.
	mux.HandleFunc("/player/", servePage("player.html"))
	mux.HandleFunc("/mission/", servePage("mission.html"))

	// The dashboard's sections. Each is a real URL so it can be linked,
	// bookmarked and opened in a tab, rather than a panel someone has to scroll
	// past on the way to the one they wanted.
	mux.HandleFunc("/missions", servePage("missions.html"))
	mux.HandleFunc("/airframes", servePage("airframes.html"))
	mux.HandleFunc("/weapons", servePage("weapons.html"))
	mux.HandleFunc("/landings", servePage("landings.html"))
	mux.HandleFunc("/log", servePage("log.html"))

	// The playground moves off / now that the dashboard lives there. It stays
	// out of production entirely: it is an unauthenticated query console.
	if development {
		mux.Handle("/playground", playground.Handler("GraphQL Playground", "/query"))
	}

	// No CORS headers are sent, and that is the policy rather than an
	// oversight: without them a browser refuses to hand another origin's page
	// the response, which is the correct answer for an API that has no
	// authentication yet. Adding permissive CORS would remove the one control
	// currently doing any work.

	logs.Sugar.Infof("Dashboard available at http://localhost:%s/", port)
	logs.Sugar.Infof("GraphQL API listening on http://localhost:%s/query", port)
	if development {
		logs.Sugar.Infof("GraphQL playground available at http://localhost:%s/playground", port)
	}

	// Say plainly what is exposed. The API has no authentication, so binding
	// every interface publishes the whole database to anything that can route
	// to this machine. That may be exactly what is wanted on a trusted LAN, but
	// it should be a decision rather than a surprise.
	if !isLoopback(host) {
		logs.Sugar.Warnf(
			"GraphQL API is reachable from the network on %s:%s with no authentication. "+
				"Set HOST=127.0.0.1 to restrict it to this machine.",
			displayHost(host), port)
	}

	if err := http.ListenAndServe(host+":"+port, mux); err != nil {
		logs.Sugar.Fatal(err)
	}
}

// servePage hands over one embedded document whatever the rest of the path is,
// so /player/2 and /mission/37 are real pages rather than client-side illusions
// that break on a refresh.
func servePage(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := web.FS().Open(name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() { _ = page.Close() }()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := io.Copy(w, page); err != nil {
			logs.Sugar.Errorf("Failed to serve %s: %v", name, err)
		}
	}
}

// isLoopback reports whether a bind host keeps the listener on this machine.
// An empty host is every interface, which is the permissive case.
func isLoopback(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

func displayHost(host string) string {
	if host == "" {
		return "all interfaces"
	}
	return host
}
