package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admin_back_go/internal/module/aichat"
	platformai "admin_back_go/internal/platform/ai"
)

func TestAIChatEngineFactorySupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	engine, err := (aiChatEngineFactory{}).NewEngine(context.Background(), aichat.EngineConfig{
		EngineType: platformai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	result, err := engine.StreamChat(context.Background(), platformai.ChatInput{
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
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-test"}]}`))
	}))
	defer server.Close()

	result, err := (aiProviderTester{}).TestConnection(context.Background(), platformai.TestConnectionInput{
		EngineType: platformai.EngineTypeOpenAI,
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
