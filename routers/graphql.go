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

	http.Handle("/query", srv)

	if environment != "production" {
		http.Handle("/", playground.Handler("GraphQL Playground", "/query"))
	}

	logs.Sugar.Infof("GraphQL server listening on port %s", port)
	if environment != "production" {
		logs.Sugar.Infof("GraphQL playground available at http://localhost:%s/", port)
	}
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		logs.Sugar.Fatal(err)
	}
}
