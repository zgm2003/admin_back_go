package dify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platformai "admin_back_go/internal/platform/ai"
)

type sink struct{ events []platformai.Event }

func (s *sink) Emit(ctx context.Context, event platformai.Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestClientStreamChatSendsDifyRequestAndMapsStream(t *testing.T) {
	var authHeader string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat-messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"event":"message","task_id":"task-1","id":"message-1","conversation_id":"conversation-1","answer":"hi"}`,
			`data: {"event":"message_end","task_id":"task-1","message_id":"message-1","conversation_id":"conversation-1","metadata":{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"total_price":"0.001","latency":0.5}}}`,
		}, "\n")))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL + "/v1", APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	s := &sink{}
	result, err := client.StreamChat(context.Background(), platformai.ChatInput{Content: "hello", UserKey: "admin:7", ConversationEngineID: "old", Inputs: map[string]any{"tenant": "admin"}}, s)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if authHeader != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", authHeader)
	}
	if requestBody["query"] != "hello" || requestBody["user"] != "admin:7" || requestBody["response_mode"] != "streaming" || requestBody["conversation_id"] != "old" {
		t.Fatalf("unexpected request body: %#v", requestBody)
	}
	if result.EngineTaskID != "task-1" || result.EngineMessageID != "message-1" || result.EngineConversationID != "conversation-1" || result.Answer != "hi" || result.TotalTokens != 3 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(s.events) != 2 || s.events[0].Type != "delta" || s.events[1].Type != "completed" {
		t.Fatalf("unexpected sink events: %#v", s.events)
	}
}

func TestClientStopChatSendsTaskAndUser(t *testing.T) {
	var authHeader string
	var requestBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat-messages/task-1/stop" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := client.StopChat(context.Background(), platformai.StopChatInput{EngineTaskID: "task-1", UserKey: "admin:7"}); err != nil {
		t.Fatalf("StopChat returned error: %v", err)
	}
	if authHeader != "Bearer test-key" || requestBody["user"] != "admin:7" {
		t.Fatalf("unexpected stop call: auth=%q body=%#v", authHeader, requestBody)
	}
}

func TestClientSyncKnowledgeCreatesDocumentByText(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/datasets/dataset-1/document/create-by-text" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"document": map[string]any{"id": "doc-1", "indexing_status": "indexing"}, "batch": "batch-1"})
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := client.SyncKnowledge(context.Background(), platformai.KnowledgeSyncInput{DatasetID: "dataset-1", Document: platformai.KnowledgeDocument{Name: "Doc", Text: "hello"}})
	if err != nil {
		t.Fatalf("SyncKnowledge returned error: %v", err)
	}
	if requestBody["name"] != "Doc" || requestBody["text"] != "hello" || result.EngineDocumentID != "doc-1" || result.EngineBatch != "batch-1" || result.IndexingStatus != "indexing" {
		t.Fatalf("unexpected sync: body=%#v result=%#v", requestBody, result)
	}
}

func TestTestConnectionUsesParametersEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parameters" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing auth header: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := client.TestConnection(context.Background(), platformai.TestConnectionInput{})
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if !result.OK || result.Status == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
