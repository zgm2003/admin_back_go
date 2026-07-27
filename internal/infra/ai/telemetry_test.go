package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"admin_back_go/internal/telemetry"
)

func TestInstrumentedEngineRecordsFirstByteTotalAndTokensWithoutProviderPayload(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	engine := InstrumentEngine("openai", "chat", fakeTelemetryEngine{}, recorder)
	result, err := engine.StreamChat(context.Background(), ChatInput{
		Content: "private prompt",
		Inputs:  map[string]any{"authorization": "Bearer private"},
	}, EventSinkFunc(func(context.Context, Event) error { return nil }))
	if err != nil || result.TotalTokens != 5 {
		t.Fatalf("result=%+v err=%v", result, err)
	}

	assertProviderEvents(t, recorder.Events(), "chat", []string{
		"provider.requests",
		"provider.first_byte_seconds",
		"provider.total_seconds",
		"provider.prompt_tokens",
		"provider.completion_tokens",
		"provider.total_tokens",
	})
}

func TestInstrumentEnginePreservesPreparedChatDispatch(t *testing.T) {
	delegate := preparedTelemetryEngine{}
	wrapped := InstrumentEngine("openai", "chat", &delegate, telemetry.Noop())
	prepared, ok := wrapped.(PreparedChatEngine)
	if !ok {
		t.Fatal("instrumented engine dropped PreparedChatEngine capability")
	}
	body, err := prepared.PrepareChat(context.Background(), ChatInput{Content: "hello"})
	if err != nil || string(body) != `{"prepared":true}` {
		t.Fatalf("prepared body=%q err=%v", body, err)
	}
	if _, err := prepared.StreamPreparedChat(context.Background(), PreparedChatRequest{Body: body, IdempotencyKey: "attempt-key"}, nil); err != nil {
		t.Fatal(err)
	}
	if delegate.key != "attempt-key" || string(delegate.body) != string(body) {
		t.Fatalf("prepared dispatch body=%q key=%q", delegate.body, delegate.key)
	}
	capabilities, ok := wrapped.(CapabilityProvider)
	if !ok || capabilities.Capabilities().SafeInputUpperBoundStrategy != SafeInputUpperBoundStrategyUTF8RequestBytesV1 {
		t.Fatal("instrumented engine dropped prepared provider capability metadata")
	}
}

func TestInstrumentedProviderModalitiesRecordOnlyBoundedMetadata(t *testing.T) {
	tests := []struct {
		name     string
		modality string
		call     func(*telemetry.MemoryRecorder) error
	}{
		{name: "image", modality: "image", call: func(recorder *telemetry.MemoryRecorder) error {
			_, err := InstrumentImageEngine("openai", fakeTelemetryImageEngine{}, recorder).GenerateImages(context.Background(), ImageInput{Prompt: "private"})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := telemetry.NewMemoryRecorder()
			if err := test.call(recorder); err != nil {
				t.Fatalf("provider call: %v", err)
			}
			assertProviderEvents(t, recorder.Events(), test.modality, []string{"provider.requests", "provider.total_seconds"})
		})
	}
}

func assertProviderEvents(t *testing.T, events []telemetry.Event, modality string, names []string) {
	t.Helper()
	seen := make(map[string]bool, len(names))
	for _, event := range events {
		if event.Attributes["provider.name"] != "openai" || event.Attributes["provider.modality"] != modality || event.Attributes["provider.status"] != "ok" {
			t.Fatalf("provider attributes mismatch: %+v", event)
		}
		seen[event.Name] = true
		for key := range event.Attributes {
			if key == "prompt" || key == "payload" || key == "authorization" {
				t.Fatalf("sensitive provider attribute retained: %+v", event)
			}
		}
	}
	for _, name := range names {
		if !seen[name] {
			t.Fatalf("missing %s in %+v", name, events)
		}
	}
	text := strings.ToLower(fmt.Sprint(events))
	for _, forbidden := range []string{"private", "authorization", "task-id"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("provider payload leaked (%s): %s", forbidden, text)
		}
	}
}

type EventSinkFunc func(context.Context, Event) error

func (function EventSinkFunc) Emit(ctx context.Context, event Event) error {
	return function(ctx, event)
}

type fakeTelemetryEngine struct{}

type preparedTelemetryEngine struct {
	body []byte
	key  string
}

func (*preparedTelemetryEngine) Capabilities() CapabilityMetadata {
	return CapabilityMetadata{SafeInputUpperBoundStrategy: SafeInputUpperBoundStrategyUTF8RequestBytesV1}
}

func (preparedTelemetryEngine) TestConnection(context.Context, TestConnectionInput) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true}, nil
}

func (preparedTelemetryEngine) StreamChat(context.Context, ChatInput, EventSink) (*ChatResult, error) {
	return &ChatResult{}, nil
}

func (e *preparedTelemetryEngine) PrepareChat(context.Context, ChatInput) ([]byte, error) {
	return []byte(`{"prepared":true}`), nil
}

func (e *preparedTelemetryEngine) StreamPreparedChat(_ context.Context, input PreparedChatRequest, _ EventSink) (*ChatResult, error) {
	e.body = append([]byte(nil), input.Body...)
	e.key = input.IdempotencyKey
	return &ChatResult{}, nil
}

func (fakeTelemetryEngine) TestConnection(context.Context, TestConnectionInput) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true, Status: "200 OK"}, nil
}

func (fakeTelemetryEngine) StreamChat(ctx context.Context, _ ChatInput, sink EventSink) (*ChatResult, error) {
	if sink == nil {
		return nil, errors.New("sink required")
	}
	if err := sink.Emit(ctx, Event{Type: "delta", DeltaText: "private provider delta"}); err != nil {
		return nil, err
	}
	return &ChatResult{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}, nil
}

type fakeTelemetryImageEngine struct{}

func (fakeTelemetryImageEngine) GenerateImages(context.Context, ImageInput) (*ImageResult, error) {
	return &ImageResult{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}, nil
}
