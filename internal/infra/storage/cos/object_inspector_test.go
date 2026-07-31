package cos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type staticObjectConfigProvider struct {
	config ObjectConfig
	err    error
	calls  int
}

func (provider *staticObjectConfigProvider) ActiveObjectConfig(context.Context) (ObjectConfig, error) {
	provider.calls++
	return provider.config, provider.err
}

func TestObjectInspectorUsesHeadMetadataAndTrustedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead || request.URL.Path != "/ai_chat_images/2026/07/28/demo.png" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "image/png; charset=binary")
		writer.Header().Set("Content-Length", "321")
		writer.Header().Set("ETag", `"image-v1"`)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	provider := &staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1", Region: "ap-test", Endpoint: server.URL,
	}}
	inspector := NewObjectInspector(provider, ObjectInspectorConfig{
		Enabled: true, Timeout: time.Second, HTTPClient: server.Client(),
	})

	metadata, err := inspector.Head(context.Background(), "ai_chat_images/2026/07/28/demo.png")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if metadata.Key != "ai_chat_images/2026/07/28/demo.png" || metadata.MIMEType != "image/png" || metadata.Size != 321 || metadata.ETag != `"image-v1"` ||
		metadata.TrustedURL != server.URL+"/ai_chat_images/2026/07/28/demo.png" {
		t.Fatalf("metadata=%#v", metadata)
	}
	if provider.calls != 1 {
		t.Fatalf("config provider calls=%d", provider.calls)
	}
}

func TestTrustedAIChatObjectKeySeparatesLegacyImagesFromNewFiles(t *testing.T) {
	tests := []struct {
		key, typ string
		wantOK   bool
	}{
		{"ai_chat_images/2026/07/old.jpg", "image", true},
		{"ai_chat_images/2026/07/old.pdf", "file", false},
		{"ai_chat_attachments/2026/07/new.jpg", "image", true},
		{"ai_chat_attachments/2026/07/report.pdf", "file", true},
		{"ai_chat_attachments/../secret.pdf", "file", false},
		{"exports/report.pdf", "file", false},
	}
	for _, test := range tests {
		t.Run(test.typ+"/"+test.key, func(t *testing.T) {
			_, err := TrustedAIChatObjectKey(test.key, test.typ)
			if (err == nil) != test.wantOK {
				t.Fatalf("key=%q type=%q err=%v", test.key, test.typ, err)
			}
		})
	}
}

func TestObjectInspectorRejectsMissingETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/pdf")
		writer.Header().Set("Content-Length", "321")
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	inspector := NewObjectInspector(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1", Region: "ap-test", Endpoint: server.URL,
	}}, ObjectInspectorConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})

	_, err := inspector.Head(context.Background(), "ai_chat_attachments/2026/07/report.pdf")
	if !errors.Is(err, ErrInvalidObjectMetadata) {
		t.Fatalf("missing ETag error=%v", err)
	}
}

func TestObjectInspectorRejectsUntrustedKeyBeforeConfigLookup(t *testing.T) {
	provider := &staticObjectConfigProvider{}
	inspector := NewObjectInspector(provider, ObjectInspectorConfig{Enabled: true})

	_, err := inspector.Head(context.Background(), "images/demo.png")
	if !errors.Is(err, ErrUntrustedObjectKey) {
		t.Fatalf("untrusted key error=%v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("untrusted key reached config provider: calls=%d", provider.calls)
	}
}
