package routers

import (
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/MartinHell/overlord/graph"
	"github.com/MartinHell/overlord/graph/generated"
	"github.com/MartinHell/overlord/logs"
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

	// Health endpoint for container probes. It is deliberately separate from the
	// playground, which is not served in production.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	if environment != "production" {
		mux.Handle("/", playground.Handler("GraphQL Playground", "/query"))
	}

	logs.Sugar.Infof("GraphQL server listening on port %s", port)
	if environment != "production" {
		logs.Sugar.Infof("GraphQL playground available at http://localhost:%s/", port)
	}
	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		logs.Sugar.Fatal(err)
	}
}
