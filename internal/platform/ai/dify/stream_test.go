package dify

import (
	"strings"
	"testing"
)

func TestParseStreamEventsAndMapResult(t *testing.T) {
	raw := strings.NewReader(strings.Join([]string{
		`event: message`,
		`data: {"event":"message","task_id":"task-1","id":"msg-1","conversation_id":"conv-1","answer":"hel"}`,
		``,
		`data: {"event":"message","task_id":"task-1","message_id":"msg-1","conversation_id":"conv-1","answer":"lo"}`,
		`data: {"event":"message_end","task_id":"task-1","message_id":"msg-1","conversation_id":"conv-1","metadata":{"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"total_price":"0.010000","latency":1.25}}}`,
	}, "\n"))

	events, err := parseStreamEvents(raw)
	if err != nil {
		t.Fatalf("parseStreamEvents returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %#v", events)
	}
	result, err := streamResult(events)
	if err != nil {
		t.Fatalf("streamResult returned error: %v", err)
	}
	if result.Answer != "hello" || result.EngineTaskID != "task-1" || result.EngineMessageID != "msg-1" || result.EngineConversationID != "conv-1" {
		t.Fatalf("unexpected result ids/content: %#v", result)
	}
	if result.PromptTokens != 2 || result.CompletionTokens != 3 || result.TotalTokens != 5 || result.Cost != 0.01 || result.LatencyMs != 1250 {
		t.Fatalf("unexpected usage: %#v", result)
	}
}

func TestStreamErrorBecomesUpstreamError(t *testing.T) {
	events, err := parseStreamEvents(strings.NewReader(`data: {"event":"error","code":"bad_request","message":"bad"}`))
	if err != nil {
		t.Fatalf("parseStreamEvents returned error: %v", err)
	}
	_, err = streamResult(events)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected stream error, got %v", err)
	}
}

func TestEventForSinkMapsDifyMessageEvents(t *testing.T) {
	event, ok := eventForSink(streamEvent{Event: "message", TaskID: "task", Answer: "hello"})
	if !ok || event.Type != "delta" || event.DeltaText != "hello" || event.Payload["task_id"] != "task" {
		t.Fatalf("unexpected message event: ok=%v event=%#v", ok, event)
	}
	completed, ok := eventForSink(streamEvent{Event: "message_end", TaskID: "task"})
	if !ok || completed.Type != "completed" {
		t.Fatalf("unexpected completed event: ok=%v event=%#v", ok, completed)
	}
}
