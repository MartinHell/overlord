package routers

import (
	"io"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/MartinHell/overlord/graph"
	"github.com/MartinHell/overlord/graph/generated"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/web"
)

func GraphQLHandler() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}))

	mux := http.NewServeMux()
	mux.Handle("/query", srv)

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

	// A pilot's record is a page at /player/<id>, so it has to be served on a
	// refresh, a bookmark or a pasted link and not only when the dashboard
	// happens to navigate there. The id is read from the path by the client;
	// the server's job is just to hand over the document.
	mux.HandleFunc("/player/", func(w http.ResponseWriter, r *http.Request) {
		page, err := web.FS().Open("player.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() { _ = page.Close() }()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := io.Copy(w, page); err != nil {
			logs.Sugar.Errorf("Failed to serve player page: %v", err)
		}
	})

	// The playground moves off / now that the dashboard lives there. It stays
	// out of production entirely: it is an unauthenticated query console.
	if environment != "production" {
		mux.Handle("/playground", playground.Handler("GraphQL Playground", "/query"))
	}

	logs.Sugar.Infof("Dashboard available at http://localhost:%s/", port)
	logs.Sugar.Infof("GraphQL API listening on http://localhost:%s/query", port)
	if environment != "production" {
		logs.Sugar.Infof("GraphQL playground available at http://localhost:%s/playground", port)
	}

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logs.Sugar.Fatal(err)
	}
}
