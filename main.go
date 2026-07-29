package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/MartinHell/overlord/controllers"
	"github.com/MartinHell/overlord/initializers"
	"github.com/MartinHell/overlord/logs"
	"github.com/MartinHell/overlord/routers"
)

func init() {
	initializers.LoadEnvVariables()

	initializers.ConnectToDB()

	if err := initializers.InitGrpc(); err != nil {
		logs.Sugar.Fatalf("Failed to configure the gRPC client: %v", err)
	}
}

func main() {
	defer initializers.GrpcClientConn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go routers.GraphQLHandler()
	go controllers.StreamEvents(ctx)

	logs.Sugar.Infoln("Server started")

	// Block until SIGINT or SIGTERM arrives; cancelling the context lets the
	// event stream unwind instead of being killed mid-reconnect.
	<-ctx.Done()

	logs.Sugar.Infoln("Server stopped")
}
