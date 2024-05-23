package models

import "github.com/DCS-gRPC/go-bindings/dcs/v0/mission"

type Stream struct {
	Stream *mission.MissionService_StreamEventsClient
}
