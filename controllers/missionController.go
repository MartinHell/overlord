package controllers

import (
	"context"
	"log"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"google.golang.org/grpc"
)

type MissionController struct{}

func (m *MissionController) InitGrpc(conn *grpc.ClientConn) (*mission.MissionService_StreamEventsClient, error) {
	// Set up the gRPC client here
	return mission.NewMissionServiceClient(conn), nil
}

func (m *MissionController) HandleEvent(event *mission.StreamEventsResponse_Event) error {
	// Call the event handling function here
	return handleEvent(event)
}
