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
		{name: "video create", modality: "video", call: func(recorder *telemetry.MemoryRecorder) error {
			_, err := InstrumentVideoEngine("openai", fakeTelemetryVideoEngine{}, recorder).CreateVideo(context.Background(), VideoInput{Prompt: "private"})
			return err
		}},
		{name: "video get", modality: "video", call: func(recorder *telemetry.MemoryRecorder) error {
			_, err := InstrumentVideoEngine("openai", fakeTelemetryVideoEngine{}, recorder).GetVideo(context.Background(), "private-task-id")
			return err
		}},
		{name: "video download", modality: "video", call: func(recorder *telemetry.MemoryRecorder) error {
			_, _, err := InstrumentVideoEngine("openai", fakeTelemetryVideoEngine{}, recorder).DownloadVideo(context.Background(), "private-task-id")
			return err
		}},
		{name: "audio", modality: "audio", call: func(recorder *telemetry.MemoryRecorder) error {
			_, err := InstrumentAudioEngine("openai", fakeTelemetryAudioEngine{}, recorder).GenerateAudio(context.Background(), AudioInput{Prompt: "private"})
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

type fakeTelemetryVideoEngine struct{}

func (fakeTelemetryVideoEngine) CreateVideo(context.Context, VideoInput) (*VideoTask, error) {
	return &VideoTask{ID: "private-upstream-id"}, nil
}

func (fakeTelemetryVideoEngine) GetVideo(context.Context, string) (*VideoTask, error) {
	return &VideoTask{ID: "private-upstream-id"}, nil
}

func (fakeTelemetryVideoEngine) DownloadVideo(context.Context, string) ([]byte, string, error) {
	return []byte("private-video"), "video/mp4", nil
}

type fakeTelemetryAudioEngine struct{}

func (fakeTelemetryAudioEngine) GenerateAudio(context.Context, AudioInput) (*AudioResult, error) {
	return &AudioResult{Body: []byte("private-audio"), ContentType: "audio/mpeg"}, nil
}
