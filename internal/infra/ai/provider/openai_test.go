package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/telemetry"
)

func TestOpenAIDriverListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-b","object":"model","created":2,"owned_by":"openai"},{"id":"gpt-a","object":"model","created":1,"owned_by":"openai"}]}`))
	}))
	defer server.Close()

	driver := NewOpenAIDriver(nil)
	models, err := driver.ListModels(context.Background(), Config{BaseURL: server.URL, APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("ListModels error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].ID != "gpt-a" || models[1].ID != "gpt-b" {
		t.Fatalf("models not sorted by id: %+v", models)
	}
}

func TestOpenAIDriverRecordsDiscoveryTelemetryWithoutCredentialsOrResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"private-model-id"}]}`))
	}))
	defer server.Close()

	recorder := telemetry.NewMemoryRecorder()
	driver := NewOpenAIDriver(server.Client(), WithTelemetry(recorder))
	models, err := driver.ListModels(context.Background(), Config{BaseURL: server.URL, APIKey: "private-api-key"})
	if err != nil || len(models) != 1 {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected discovery count and duration, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["provider.name"] != DriverOpenAI || event.Attributes["provider.modality"] != "discovery" || event.Attributes["provider.status"] != "ok" {
			t.Fatalf("provider discovery telemetry mismatch: %+v", event)
		}
	}
	if text := strings.ToLower(fmt.Sprint(events)); strings.Contains(text, "private") || strings.Contains(text, "model-id") {
		t.Fatalf("provider credential/response leaked: %s", text)
	}
}

func TestOpenAIDriverAppendsVersionPathForOriginOnlyBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-a","object":"model","created":1,"owned_by":"openai"}]}`))
	}))
	defer server.Close()

	driver := NewOpenAIDriver(nil)
	models, err := driver.ListModels(context.Background(), Config{BaseURL: server.URL, APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("ListModels error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-a" {
		t.Fatalf("models = %+v", models)
	}
}

func TestOpenAIDriverRejectsMissingAPIKey(t *testing.T) {
	driver := NewOpenAIDriver(nil)
	_, err := driver.ListModels(context.Background(), Config{})
	if err == nil || !strings.Contains(err.Error(), "missing OpenAI API key") {
		t.Fatalf("error = %v, want missing OpenAI API key", err)
	}
}

func TestOpenAIDriverDoesNotLeakAPIKeyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	driver := NewOpenAIDriver(nil)
	_, err := driver.ListModels(context.Background(), Config{BaseURL: server.URL, APIKey: "sk-secret-value"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-value") {
		t.Fatalf("error leaked api key: %v", err)
	}
}
