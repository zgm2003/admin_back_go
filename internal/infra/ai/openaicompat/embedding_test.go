package openaicompat

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestEmbeddingValidatesRequestAndResponseContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Method != http.MethodPost {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "embed-v1" || !reflect.DeepEqual(body.Input, []string{"one", "two"}) {
			t.Fatalf("body=%#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"embed-v1","data":[{"object":"embedding","index":0,"embedding":[1,2,3]},{"object":"embedding","index":1,"embedding":[4,5,6]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`))
	}))
	defer server.Close()

	client, err := NewEmbeddingClient(Config{BaseURL: server.URL + "/v1", APIKey: "secret", HTTPClient: server.Client()}, "embed-v1",
		infraai.EmbeddingCapabilities{Dimensions: 3, MaxInputs: 2, MaxInputTokens: 20, TokenCounterID: infraai.TokenCounterUTF8BytesV1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Embed(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelID != "embed-v1" || result.Usage.PromptTokens != 4 || !reflect.DeepEqual(result.Vectors, [][]float32{{1, 2, 3}, {4, 5, 6}}) {
		t.Fatalf("result=%#v", result)
	}
}

func TestEmbeddingRejectsInvalidProviderFacts(t *testing.T) {
	for _, response := range []string{
		`{"model":"other","data":[{"index":0,"embedding":[1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`,
		`{"model":"embed-v1","data":[{"index":0,"embedding":[1,2]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`,
		`{"model":"embed-v1","data":[{"index":0,"embedding":[1e999]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(response)) }))
		client, newErr := NewEmbeddingClient(Config{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()}, "embed-v1",
			infraai.EmbeddingCapabilities{Dimensions: 1, MaxInputs: 1, MaxInputTokens: 1, TokenCounterID: infraai.TokenCounterUTF8BytesV1})
		if newErr != nil {
			t.Fatal(newErr)
		}
		_, err := client.Embed(context.Background(), []string{"x"})
		server.Close()
		if err == nil {
			t.Fatalf("response %s was accepted", response)
		}
	}
	_ = math.MaxFloat32
}

func TestEmbeddingRejectsResponseLargerThanDeclaredShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 2<<20))
	}))
	defer server.Close()
	client, err := NewEmbeddingClient(Config{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()}, "embed-v1",
		infraai.EmbeddingCapabilities{Dimensions: 1, MaxInputs: 1, MaxInputTokens: 1, TokenCounterID: infraai.TokenCounterUTF8BytesV1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("oversized embedding response was accepted")
	}
}
