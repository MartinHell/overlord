package controllers

import (
	"models"
)

type EventHandler interface {
	HandleEvent(event *mission.StreamEventsResponse_Event) error
}

func handleEvent(event *mission.StreamEventsResponse_Event) error {
	// Handle the event here, using the event handler interface
	return nil
}
