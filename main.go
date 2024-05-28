package main

import (
	"os"
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
	initializers.InitGrpc()
}

func main() {

	/*	r := gin.Default()

	 	r.Use(middleware.LoggerMiddleware())

		routers.Route(r) */

	go routers.GraphQLHandler()

	// Create a channel to listen for OS signals
	sigs := make(chan os.Signal, 1)

	// Channel to wait for the done signal
	done := make(chan bool, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go controllers.StreamEvents()

	/* 	go r.Run() */

	defer initializers.GrpcClientConn.Close()

	go func() {
		sig := <-sigs // This will block the program until a signal is received
		logs.Sugar.Infoln("Signal received: ", sig)
		done <- true
	}()

	logs.Sugar.Infoln("Server started")

	// Wait here until we receive the done signal
	<-done

	logs.Sugar.Infoln("Server stopped")
}
