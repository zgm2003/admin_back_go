package cos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"admin_back_go/internal/infra/storage"
)

func TestImagePreviewerVerifiesPersistedVersionBeforeSigning(t *testing.T) {
	var headCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead || request.URL.Path != "/ai_chat_images/2026/08/reference.png" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("If-Match") != `"image-v1"` {
			t.Fatalf("If-Match=%q", request.Header.Get("If-Match"))
		}
		headCalls.Add(1)
		writer.Header().Set("Content-Type", "image/png")
		writer.Header().Set("Content-Length", "342460")
		writer.Header().Set("ETag", `"image-v1"`)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previewer := NewImagePreviewer(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}}, ImagePreviewerConfig{Enabled: true, Timeout: time.Second, TTL: 5 * time.Minute, HTTPClient: server.Client()})
	result, err := previewer.Preview(context.Background(), storage.ImagePreviewInput{
		StorageProvider: "cos",
		ObjectKey:       "ai_chat_images/2026/08/reference.png",
		ETag:            `"image-v1"`,
		Size:            342460,
		MIMEType:        "image/png",
	})
	if err != nil {
		t.Fatalf("Preview returned error: %v", err)
	}
	if headCalls.Load() != 1 || result.ExpiresIn != 300 {
		t.Fatalf("head calls=%d result=%+v", headCalls.Load(), result)
	}
	if !strings.HasPrefix(result.URL, server.URL+"/ai_chat_images/2026/08/reference.png?") ||
		!strings.Contains(result.URL, "q-signature=") {
		t.Fatalf("signed URL=%q", result.URL)
	}
}

func TestImagePreviewerRejectsChangedOrUntrustedObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()
	previewer := NewImagePreviewer(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}}, ImagePreviewerConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})

	_, err := previewer.Preview(context.Background(), storage.ImagePreviewInput{
		StorageProvider: "cos", ObjectKey: "ai_chat_images/changed.png", ETag: `"v1"`, Size: 1, MIMEType: "image/png",
	})
	if !errors.Is(err, storage.ErrConditionalObjectVersionChanged) {
		t.Fatalf("changed object error=%v", err)
	}

	_, err = previewer.Preview(context.Background(), storage.ImagePreviewInput{
		StorageProvider: "cos", ObjectKey: "outside/image.png", ETag: `"v1"`, Size: 1, MIMEType: "image/png",
	})
	if !errors.Is(err, storage.ErrInvalidImagePreviewInput) {
		t.Fatalf("untrusted object error=%v", err)
	}
}
