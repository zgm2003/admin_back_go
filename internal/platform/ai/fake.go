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

	Stopped []StopChatInput
	Synced  []KnowledgeSyncInput
	Status  []KnowledgeStatusInput
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

func (e *FakeEngine) StopChat(ctx context.Context, input StopChatInput) error {
	if e != nil && e.Err != nil {
		return e.Err
	}
	if e != nil {
		e.Stopped = append(e.Stopped, input)
	}
	return nil
}

func (e *FakeEngine) SyncKnowledge(ctx context.Context, input KnowledgeSyncInput) (*KnowledgeSyncResult, error) {
	if e != nil && e.Err != nil {
		return nil, e.Err
	}
	if e != nil {
		e.Synced = append(e.Synced, input)
	}
	return &KnowledgeSyncResult{
		EngineDatasetID:  nonEmpty(input.DatasetID, "fake-dataset"),
		EngineDocumentID: "fake-document",
		EngineBatch:      "fake-batch",
		IndexingStatus:   "completed",
	}, nil
}

func (e *FakeEngine) KnowledgeStatus(ctx context.Context, input KnowledgeStatusInput) (*KnowledgeStatusResult, error) {
	if e != nil && e.Err != nil {
		return nil, e.Err
	}
	if e != nil {
		e.Status = append(e.Status, input)
	}
	return &KnowledgeStatusResult{IndexingStatus: "completed"}, nil
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
