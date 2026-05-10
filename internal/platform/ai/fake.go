package ai

import (
	"context"
	"strings"
	"time"
)

type FakeEngine struct {
	Answer string
	Err    error
	Now    func() time.Time
}

func NewFakeEngine(answer string) *FakeEngine {
	return &FakeEngine{Answer: answer}
}

func (e *FakeEngine) TestConnection(ctx context.Context, input TestConnectionInput) (*TestConnectionResult, error) {
	if e != nil && e.Err != nil {
		return nil, e.Err
	}
	return &TestConnectionResult{OK: true, Status: "ok", Message: "fake engine ready"}, nil
}

func (e *FakeEngine) StreamChat(ctx context.Context, input ChatInput, sink EventSink) (*ChatResult, error) {
	if e != nil && e.Err != nil {
		return nil, e.Err
	}
	answer := "fake response"
	if e != nil && strings.TrimSpace(e.Answer) != "" {
		answer = e.Answer
	}
	if sink != nil {
		if err := sink.Emit(ctx, Event{Type: "start", Payload: map[string]any{"run_id": input.RunID}}); err != nil {
			return nil, err
		}
		if err := sink.Emit(ctx, Event{Type: "delta", DeltaText: answer, Payload: map[string]any{"run_id": input.RunID, "delta": answer}}); err != nil {
			return nil, err
		}
		if err := sink.Emit(ctx, Event{Type: "completed", Payload: map[string]any{"run_id": input.RunID}}); err != nil {
			return nil, err
		}
	}
	return &ChatResult{
		EngineConversationID: nonEmpty(input.ConversationEngineID, "fake-conversation"),
		EngineMessageID:      "fake-message",
		EngineTaskID:         "fake-task",
		Answer:               answer,
		PromptTokens:         1,
		CompletionTokens:     1,
		TotalTokens:          2,
	}, nil
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
