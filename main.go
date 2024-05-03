package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/MartinHell/overlord/controllers"
	"github.com/MartinHell/overlord/initializers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	initializers.LoadEnvVariables()

	initializers.ConnectToDB()
}

func connectGRPC() {
	var addr = os.Getenv("GRPC_SERVER_ADDRESS")

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		log.Panicf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	log.Print("Connected to server")

	stream, err := controllers.MissionClientController(conn)
	if err != nil {
		log.Panicf("Failed to get mission client: %v", err)
	}

	log.Print("Got mission client")
	controllers.StreamEvents(*stream)
}

func main() {

	connectGRPC()

	/* r := gin.Default()

	routers.Route(r)

	r.Run() */
}
