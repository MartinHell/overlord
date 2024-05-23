package initializers

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var NetServiceClient net.NetServiceClient

var StreamEventsClient mission.MissionService_StreamEventsClient

var GrpcClientConn *grpc.ClientConn

func InitGrpc() {

	var addr = os.Getenv("GRPC_SERVER_ADDRESS")

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	GrpcClientConn, err = grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		log.Panicf("Failed to connect to server: %v", err)
	}

	log.Print("Connected to server")

	missionClient := mission.NewMissionServiceClient(GrpcClientConn)
	StreamEventsClient, err = missionClient.StreamEvents(context.Background(), &mission.StreamEventsRequest{})
	if err != nil {
		log.Panicf("Failed to open events stream: %v", err)
	}

	log.Print("Got mission client")

	NetServiceClient = net.NewNetServiceClient(GrpcClientConn) // Fix: Create a new instance of net.NetServiceClient
	if err != nil {
		log.Panicf("Failed to create NetServiceClient: %v", err)
	}

	log.Print("Got net client")
}
