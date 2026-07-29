package controllers

import (
	"errors"
	"testing"
	"time"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
)

type stubHandler struct {
	err    error
	panics bool
	calls  int
}

func (s *stubHandler) HandleEvent(event *mission.StreamEventsResponse) error {
	s.calls++
	if s.panics {
		panic("handler exploded")
	}
	return s.err
}

func TestHandleEventSafelyRecoversPanic(t *testing.T) {
	handler := &stubHandler{panics: true}

	err := handleEventSafely(handler, &mission.StreamEventsResponse{})

	if err == nil {
		t.Fatal("expected an error when the handler panics, got nil")
	}
	if handler.calls != 1 {
		t.Fatalf("expected the handler to be called once, got %d", handler.calls)
	}
}

func TestHandleEventSafelyPassesThroughError(t *testing.T) {
	want := errors.New("boom")
	handler := &stubHandler{err: want}

	got := handleEventSafely(handler, &mission.StreamEventsResponse{})

	if !errors.Is(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestHandleEventSafelyNilEventDoesNotPanic(t *testing.T) {
	// The recover path formats the event type, so a nil event must not turn a
	// handler panic into a second panic inside the deferred function.
	handler := &stubHandler{panics: true}

	if err := handleEventSafely(handler, nil); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestExponentialBackoff(t *testing.T) {
	base := 2 * time.Second
	max := 60 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 2 * time.Second},
		{attempt: 1, want: 4 * time.Second},
		{attempt: 2, want: 8 * time.Second},
		{attempt: 3, want: 16 * time.Second},
		{attempt: 5, want: max},
		{attempt: 1000, want: max},
		{attempt: -1, want: 2 * time.Second},
	}

	for _, tt := range tests {
		if got := exponentialBackoff(tt.attempt, base, max); got != tt.want {
			t.Errorf("exponentialBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
