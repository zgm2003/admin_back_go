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

func TestFakeEngineRecordsStopAndKnowledgeSync(t *testing.T) {
	engine := NewFakeEngine("hello")
	if err := engine.StopChat(context.Background(), StopChatInput{EngineTaskID: "task", UserKey: "admin:7"}); err != nil {
		t.Fatalf("StopChat returned error: %v", err)
	}
	if len(engine.Stopped) != 1 || engine.Stopped[0].EngineTaskID != "task" || engine.Stopped[0].UserKey != "admin:7" {
		t.Fatalf("unexpected stopped calls: %#v", engine.Stopped)
	}

	result, err := engine.SyncKnowledge(context.Background(), KnowledgeSyncInput{DatasetID: "dataset", Document: KnowledgeDocument{Name: "doc", Text: "body"}})
	if err != nil {
		t.Fatalf("SyncKnowledge returned error: %v", err)
	}
	if result.EngineDatasetID != "dataset" || result.EngineDocumentID == "" || result.IndexingStatus != "completed" {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if len(engine.Synced) != 1 || engine.Synced[0].Document.Name != "doc" {
		t.Fatalf("unexpected synced calls: %#v", engine.Synced)
	}
}
