package ai

import (
	"context"
	"errors"
	"testing"
)

type captureSink struct {
	events []Event
}

func (s *captureSink) Emit(ctx context.Context, event Event) error {
	s.events = append(s.events, event)
	return nil
}

type errorSink struct{}

func (errorSink) Emit(ctx context.Context, event Event) error { return errors.New("sink down") }

func TestFakeEngineStreamsStableEventsAndResult(t *testing.T) {
	engine := NewFakeEngine("hello")
	sink := &captureSink{}

	result, err := engine.StreamChat(context.Background(), ChatInput{RunID: 7, ConversationEngineID: "existing-conv"}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.EngineConversationID != "existing-conv" || result.EngineTaskID == "" || result.EngineMessageID == "" || result.Answer != "hello" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(sink.events) != 3 || sink.events[0].Type != "start" || sink.events[1].Type != "delta" || sink.events[2].Type != "completed" {
		t.Fatalf("unexpected events: %#v", sink.events)
	}
}

func TestFakeEnginePropagatesSinkError(t *testing.T) {
	_, err := NewFakeEngine("hello").StreamChat(context.Background(), ChatInput{RunID: 7}, errorSink{})
	if err == nil || err.Error() != "sink down" {
		t.Fatalf("expected sink error, got %v", err)
	}
}
