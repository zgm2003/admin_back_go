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
	if metadata.Key != "ai_chat_images/2026/07/28/demo.png" || metadata.MIMEType != "image/png" || metadata.Size != 321 ||
		metadata.TrustedURL != server.URL+"/ai_chat_images/2026/07/28/demo.png" {
		t.Fatalf("metadata=%#v", metadata)
	}
	if provider.calls != 1 {
		t.Fatalf("config provider calls=%d", provider.calls)
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
