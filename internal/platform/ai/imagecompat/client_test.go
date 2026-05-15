package imagecompat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformai "admin_back_go/internal/platform/ai"
)

func TestClientGenerateImagesSendsGenerationRequestAndParsesB64(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("path = %s, want /images/generations", r.URL.Path)
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
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), platformai.ImageInput{
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
}

func TestClientGenerateImagesSendsEditMultipartRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Fatalf("path = %s, want /images/edits", r.URL.Path)
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/out.jpg"}]}`))
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), platformai.ImageInput{
		Model:        "gpt-image-2",
		Prompt:       "edit it",
		Size:         "auto",
		Quality:      "auto",
		OutputFormat: "jpeg",
		Moderation:   "low",
		InputAssets: []platformai.ImageAsset{
			{Name: "a.png", MimeType: "image/png", Data: []byte("a")},
			{Name: "b.png", MimeType: "image/png", Data: []byte("b")},
		},
		MaskAsset: &platformai.ImageAsset{Name: "mask.png", MimeType: "image/png", Data: []byte("mask")},
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

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).GenerateImages(context.Background(), platformai.ImageInput{
		Model:  "gpt-image-2",
		Prompt: "draw",
	})
	if !errors.Is(err, platformai.ErrUpstreamFailed) {
		t.Fatalf("expected upstream failed on garbage response, got %v", err)
	}
}

func TestClientGenerateImagesDoesNotLeakAPIKeyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key sk-secret-value"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-secret-value", Timeout: time.Second}).GenerateImages(context.Background(), platformai.ImageInput{
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
