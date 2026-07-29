package routers

import (
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
	mux.Handle("/", http.FileServer(http.FS(web.FS())))

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
