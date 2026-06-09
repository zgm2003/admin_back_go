package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
)

func TestAIChatEngineFactorySupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	engine, err := (aiChatEngineFactory{}).NewEngine(context.Background(), aichat.EngineConfig{
		EngineType: infraai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	result, err := engine.StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-test"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "ok" {
		t.Fatalf("answer = %q, want ok", result.Answer)
	}
}

func TestAIProviderTesterSupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-test"}]}`))
	}))
	defer server.Close()

	result, err := (aiProviderTester{}).TestConnection(context.Background(), infraai.TestConnectionInput{
		EngineType: infraai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
		TimeoutMs:  int(time.Second / time.Millisecond),
	})
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if result == nil || !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAIAudioEngineFactorySupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path = %s, want /v1/audio/speech", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	engine, err := (aiAudioEngineFactory{}).NewAudioEngine(context.Background(), aiaudio.EngineConfig{
		EngineType: infraai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
	})
	if err != nil {
		t.Fatalf("NewAudioEngine returned error: %v", err)
	}
	result, err := engine.GenerateAudio(context.Background(), infraai.AudioInput{Model: "tts-1", Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateAudio returned error: %v", err)
	}
	if string(result.Body) != "audio" || result.ContentType != "audio/mpeg" {
		t.Fatalf("unexpected audio result: %#v", result)
	}
}
