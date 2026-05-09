package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIDriverListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
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
