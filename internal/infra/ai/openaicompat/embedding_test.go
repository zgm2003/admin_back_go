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

	client := New(Config{BaseURL: server.URL + "/v1", APIKey: "secret", HTTPClient: server.Client()})
	result, err := client.Embed(context.Background(), infraai.EmbeddingInput{
		ModelID: "embed-v1", Texts: []string{"one", "two"},
		Capabilities: infraai.EmbeddingCapabilities{Dimensions: 3, MaxInputs: 2, MaxInputTokens: 20, TokenCounterID: infraai.TokenCounterUTF8BytesV1},
	})
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
		client := New(Config{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()})
		_, err := client.Embed(context.Background(), infraai.EmbeddingInput{
			ModelID: "embed-v1", Texts: []string{"x"},
			Capabilities: infraai.EmbeddingCapabilities{Dimensions: 1, MaxInputs: 1, MaxInputTokens: 1, TokenCounterID: infraai.TokenCounterUTF8BytesV1},
		})
		server.Close()
		if err == nil {
			t.Fatalf("response %s was accepted", response)
		}
	}
	_ = math.MaxFloat32
}
