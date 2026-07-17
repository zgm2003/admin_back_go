package telemetry

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRecorderSanitizesCountObserveAndSpanEvents(t *testing.T) {
	recorder := NewMemoryRecorder()
	recorder.Count("http.requests", 1, Attributes{
		"http.method": "GET",
		"user_id":     99,
	})
	recorder.Observe("http.duration", 0.25, Attributes{"http.route": "/health"})
	ctx, finish := recorder.Start(context.Background(), "provider.request", Attributes{"provider.name": "openai", "prompt": "secret"})
	if ctx == nil {
		t.Fatal("Start returned nil context")
	}
	finish(errors.New("provider unavailable"))

	events := recorder.Events()
	if len(events) != 4 {
		t.Fatalf("events=%+v", events)
	}
	if events[0].Kind != EventCount || events[0].Attributes["http.method"] != "GET" {
		t.Fatalf("count event=%+v", events[0])
	}
	if _, exists := events[0].Attributes["user_id"]; exists {
		t.Fatalf("high-cardinality label retained: %+v", events[0])
	}
	if events[2].Kind != EventStart || events[3].Kind != EventEnd || events[3].Attributes["outcome"] != "error" {
		t.Fatalf("span events=%+v", events[2:])
	}
	if _, exists := events[2].Attributes["prompt"]; exists {
		t.Fatalf("prompt retained: %+v", events[2])
	}
}

func TestNoopRecorderImplementsRecorder(t *testing.T) {
	var recorder Recorder = Noop()
	recorder.Count("ignored", 1, nil)
	recorder.Observe("ignored", 1, nil)
	_, finish := recorder.Start(context.Background(), "ignored", nil)
	finish(nil)
}
