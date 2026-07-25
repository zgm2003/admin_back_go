package imagecompat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

func TestClientGenerateImagesSendsGenerationRequestAndParsesB64(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s, want /v1/images/generations", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"size":"1024x1024","quality":"high","output_format":"png","n":2,"data":[{"b64_json":"aW1hZ2U=","revised_prompt":"rev"}]}`))
	}))
	defer server.Close()

	compression := 80
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:             "gpt-image-2",
		Prompt:            "draw a cat",
		Size:              "1024x1024",
		Quality:           "high",
		OutputFormat:      "png",
		OutputCompression: &compression,
		Moderation:        "auto",
		N:                 2,
	})
	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
	if requestBody["model"] != "gpt-image-2" || requestBody["prompt"] != "draw a cat" || requestBody["size"] != "1024x1024" || requestBody["quality"] != "high" || requestBody["n"] != float64(2) {
		t.Fatalf("unexpected generation request: %#v", requestBody)
	}
	if len(result.Images) != 1 || result.Images[0].B64JSON != "aW1hZ2U=" || result.Images[0].RevisedPrompt != "rev" || result.Images[0].MimeType != "image/png" {
		t.Fatalf("unexpected parsed image result: %#v", result)
	}
	if result.ActualParams["size"] != "1024x1024" || result.ActualParams["n"] != 2 {
		t.Fatalf("actual params not parsed: %#v", result.ActualParams)
	}
	if result.UsageStatus != infraai.UsageStatusUnavailable {
		t.Fatalf("missing image usage must be explicit unavailable, got %q", result.UsageStatus)
	}
}

func TestClientGenerateImagesAppendsVersionPathForOriginOnlyBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s, want /v1/images/generations", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:  "gpt-image-2",
		Prompt: "draw a cat",
	})
	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
}

func TestClientGenerateImagesParsesUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":11,"output_tokens":13,"total_tokens":24}}`))
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:        "gpt-image-2",
		Prompt:       "draw a cat",
		OutputFormat: "png",
	})

	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
	if result.UsageStatus != infraai.UsageStatusReported || result.PromptTokens != 11 || result.CompletionTokens != 13 || result.TotalTokens != 24 {
		t.Fatalf("image usage not parsed from provider response: %#v", result)
	}
}

func TestClientGenerateImagesParsesCompleteJSONBeforeConnectionClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s, want /v1/images/generations", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}],"usage":{"total_tokens":1}}`))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:        "gpt-image-2",
		Prompt:       "draw a cat",
		Size:         "1024x1024",
		Quality:      "low",
		OutputFormat: "png",
		N:            1,
	})
	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
	if len(result.Images) != 1 || result.Images[0].B64JSON != "aW1hZ2U=" {
		t.Fatalf("unexpected parsed image result: %#v", result)
	}
	if result.UsageStatus != infraai.UsageStatusUnavailable {
		t.Fatalf("incomplete usage object must fail closed: %#v", result)
	}
}

func TestClientGenerateImagesFailsClosedForOmittedUsageCountsAndCapturesEvidence(t *testing.T) {
	body := []byte(`{"data":[{"b64_json":"aW1hZ2U="}],"usage":{}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "image-request-123")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:  "gpt-image-2",
		Prompt: "draw a cat",
	})
	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
	if result.UsageStatus != infraai.UsageStatusUnavailable || len(result.Usage.Items) != 0 {
		t.Fatalf("omitted image usage counts must remain unavailable: %#v", result)
	}
	if result.ProviderRequestID != "image-request-123" || result.DispatchState != infraai.DispatchStateDispatched {
		t.Fatalf("missing successful dispatch evidence: %#v", result)
	}
	if want := sha256.Sum256(body); result.ResponseSHA256 != want {
		t.Fatalf("response hash = %x, want %x", result.ResponseSHA256, want)
	}
}

func TestClientGenerateImagesSendsEditMultipartRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %s, want /v1/images/edits", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "multipart/form-data") {
			t.Fatalf("content-type = %q, want multipart/form-data", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Fatalf("model field = %q", got)
		}
		if got := r.FormValue("prompt"); got != "edit it" {
			t.Fatalf("prompt field = %q", got)
		}
		if got := r.FormValue("output_format"); got != "jpeg" {
			t.Fatalf("output_format field = %q", got)
		}
		if len(r.MultipartForm.File["image"]) != 2 {
			t.Fatalf("expected two image files, got %#v", r.MultipartForm.File)
		}
		if len(r.MultipartForm.File["mask"]) != 1 {
			t.Fatalf("expected one mask file, got %#v", r.MultipartForm.File)
		}
		for _, file := range append(r.MultipartForm.File["image"], r.MultipartForm.File["mask"]...) {
			if got := file.Header.Get("Content-Type"); got != "image/png" {
				t.Fatalf("multipart file content-type = %q, want image/png", got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/out.jpg"}]}`))
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:        "gpt-image-2",
		Prompt:       "edit it",
		Size:         "auto",
		Quality:      "auto",
		OutputFormat: "jpeg",
		Moderation:   "low",
		InputAssets: []infraai.ImageAsset{
			{Name: "a.png", MimeType: "image/png", Data: []byte("a")},
			{Name: "b.png", MimeType: "image/png", Data: []byte("b")},
		},
		MaskAsset: &infraai.ImageAsset{Name: "mask.png", MimeType: "image/png", Data: []byte("mask")},
	})
	if err != nil {
		t.Fatalf("GenerateImages returned error: %v", err)
	}
	if len(result.Images) != 1 || result.Images[0].URL != "https://cdn.example/out.jpg" || result.Images[0].MimeType != "image/jpeg" {
		t.Fatalf("unexpected parsed URL result: %#v", result)
	}
}

func TestClientGenerateImagesRejectsGarbageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"revised_prompt":"no image"}]}`))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:  "gpt-image-2",
		Prompt: "draw",
	})
	if !errors.Is(err, infraai.ErrUpstreamFailed) {
		t.Fatalf("expected upstream failed on garbage response, got %v", err)
	}
}

func TestClientGenerateImagesDoesNotLeakAPIKeyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key sk-secret-value"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-secret-value", Timeout: time.Second}).GenerateImages(context.Background(), infraai.ImageInput{
		Model:  "gpt-image-2",
		Prompt: "draw",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-value") {
		t.Fatalf("error leaked api key: %v", err)
	}
}
