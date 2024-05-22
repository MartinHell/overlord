package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/MartinHell/overlord/controllers"
	"github.com/MartinHell/overlord/initializers"
)

func init() {
	initializers.LoadEnvVariables()

	initializers.ConnectToDB()
	initializers.InitGrpc()
}

func main() {

	/* r := gin.Default()

	routers.Route(r)

	r.Run() */

	// Create a channel to listen for OS signals
	sigs := make(chan os.Signal, 1)

	// Channel to wait for the done signal
	done := make(chan bool, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go controllers.StreamEvents()

	defer initializers.GrpcClientConn.Close()

	/* 	var playercache models.PlayerCache
	   	err := playercache.RefreshPlayersCache()
	   	if err != nil {
	   		log.Panicf("Failed to refresh player cache: %v", err)
	   	}
	   	player := playercache.FindPlayerByName("Sakura")
	   	log.Printf("Player: %+v", player) */

	go func() {
		sig := <-sigs // This will block the program until a signal is received
		log.Println("Signal received: ", sig)
		done <- true
	}()

	log.Println("Server started")

	// Wait here until we receive the done signal
	<-done

	log.Println("Server stopped")
}
