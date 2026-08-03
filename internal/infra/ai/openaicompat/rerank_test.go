package openaicompat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestRerankPostsOneRequestAndMapsIndicesToCandidateIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"rerank-v1","results":[{"index":1,"relevance_score":0.9},{"index":0,"relevance_score":0.8}],"usage":{"total_tokens":12}}`))
	}))
	defer server.Close()

	client, err := NewRerankClient(Config{BaseURL: server.URL + "/v1", APIKey: "secret", HTTPClient: server.Client()}, "rerank-v1",
		infraai.RerankCapabilities{MaxDocuments: 2, MaxInputTokens: 128, TokenCounterID: infraai.TokenCounterUTF8BytesV1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Rerank(context.Background(), "refund", []infraai.RerankDocument{{CandidateID: "a", Text: "A"}, {CandidateID: "b", Text: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Scores) != 2 || result.Scores[0].CandidateID != "b" || result.Scores[1].CandidateID != "a" {
		t.Fatalf("scores=%+v", result.Scores)
	}
}

func TestRerankRejectsMissingDuplicateAndMismatchedResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"model":"rerank-v1","results":[{"index":0,"relevance_score":0.8}]}`},
		{name: "duplicate", body: `{"model":"rerank-v1","results":[{"index":0,"relevance_score":0.8},{"index":0,"relevance_score":0.7}]}`},
		{name: "model", body: `{"model":"other","results":[{"index":0,"relevance_score":0.8},{"index":1,"relevance_score":0.7}]}`},
		{name: "range", body: `{"model":"rerank-v1","results":[{"index":0,"relevance_score":1.1},{"index":1,"relevance_score":0.7}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewRerankClient(Config{BaseURL: server.URL, HTTPClient: server.Client()}, "rerank-v1",
				infraai.RerankCapabilities{MaxDocuments: 2, MaxInputTokens: 128, TokenCounterID: infraai.TokenCounterUTF8BytesV1})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Rerank(context.Background(), "refund", []infraai.RerankDocument{{CandidateID: "a", Text: "A"}, {CandidateID: "b", Text: "B"}})
			if !errors.Is(err, infraai.ErrRerankFailed) || strings.Contains(err.Error(), test.body) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
