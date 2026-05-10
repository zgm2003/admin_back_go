package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformai "admin_back_go/internal/platform/ai"
)

type captureSink struct {
	events []platformai.Event
}

func (s *captureSink) Emit(ctx context.Context, event platformai.Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestClientStreamChatParsesSSEChunksAndEmitsEveryDelta(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	sink := &captureSink{}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), platformai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-5.4"},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if requestBody["stream"] != true {
		t.Fatalf("stream = %#v, want true", requestBody["stream"])
	}
	if result.Answer != "你好" || result.PromptTokens != 2 || result.CompletionTokens != 2 || result.TotalTokens != 4 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(sink.events) != 2 || sink.events[0].DeltaText != "你" || sink.events[1].DeltaText != "好" {
		t.Fatalf("unexpected sink events: %#v", sink.events)
	}
}

func TestClientStreamChatSendsOpenAIChatCompletionAndEmitsDelta(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好，我是测试助手\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	sink := &captureSink{}
	client := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second})
	result, err := client.StreamChat(context.Background(), platformai.ChatInput{
		Content: "你是谁",
		Inputs: map[string]any{
			"model_id":      "gpt-5.4",
			"system_prompt": "你是一个后台助手",
			"history": []map[string]string{
				{"role": "user", "content": "上一轮用户"},
				{"role": "assistant", "content": "上一轮助手"},
			},
		},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "你好，我是测试助手" || result.PromptTokens != 2 || result.CompletionTokens != 3 || result.TotalTokens != 5 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(sink.events) != 1 || sink.events[0].Type != "delta" || sink.events[0].DeltaText != "你好，我是测试助手" {
		t.Fatalf("unexpected sink events: %#v", sink.events)
	}
	if requestBody["model"] != "gpt-5.4" || requestBody["stream"] != true {
		t.Fatalf("unexpected model/stream: %#v", requestBody)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 4 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}
	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, want := range wantRoles {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != want {
			t.Fatalf("message[%d] = %#v, want role %s", i, messages[i], want)
		}
	}
}

func TestClientStreamChatDoesNotSendSystemMessageWhenSystemPromptBlank(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), platformai.ChatInput{
		Content: "你是谁",
		Inputs: map[string]any{
			"model_id":      "gpt-5.4",
			"system_prompt": "   ",
		},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["role"] != "user" {
		t.Fatalf("blank system prompt must not produce system message: %#v", messages)
	}
}

func TestClientStreamChatSendsVisionContentAndRuntimeParams(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"看到了\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), platformai.ChatInput{
		Content: "看图",
		Inputs: map[string]any{
			"model_id":    "gpt-5.4",
			"temperature": 0.7,
			"max_tokens":  1024.0,
			"attachments": []any{map[string]any{"type": "image", "url": "https://example.test/a.png"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if requestBody["temperature"] != 0.7 || requestBody["max_tokens"] != 1024.0 {
		t.Fatalf("runtime params not sent: %#v", requestBody)
	}
	messages := requestBody["messages"].([]any)
	userMessage := messages[len(messages)-1].(map[string]any)
	parts, ok := userMessage["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("vision content not sent: %#v", userMessage["content"])
	}
	if parts[0].(map[string]any)["type"] != "text" || parts[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("unexpected content parts: %#v", parts)
	}
}

func TestClientDoesNotLeakAPIKeyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key sk-secret-value"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-secret-value", Timeout: time.Second}).
		StreamChat(context.Background(), platformai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-value") {
		t.Fatalf("error leaked api key: %v", err)
	}
}
