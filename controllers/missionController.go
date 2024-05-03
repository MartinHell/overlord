package controllers

import (
	"context"
	"log"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"google.golang.org/grpc"
)

func MissionClientController(conn *grpc.ClientConn) (*mission.MissionService_StreamEventsClient, error) {

	missionClient := mission.NewMissionServiceClient(conn)
	stream, err := missionClient.StreamEvents(context.Background(), &mission.StreamEventsRequest{})
	if err != nil {
		log.Panicf("Failed to open events stream: %v", err)
	}

	return &stream, nil
}
