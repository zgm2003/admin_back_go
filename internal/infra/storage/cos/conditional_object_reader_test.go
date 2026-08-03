package cos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admin_back_go/internal/infra/storage"
)

func TestConditionalObjectReaderUsesIfMatchAndValidatesFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-Match") != `"v1"` {
			http.Error(w, "precondition", http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "4")
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(w, "data")
		}
	}))
	defer server.Close()

	reader := NewConditionalObjectReader(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "id", SecretKey: "key", Bucket: "bucket", Region: "test", Endpoint: server.URL,
	}}, ObjectStreamerConfig{Enabled: true, Timeout: time.Second, HTTPClient: http.DefaultClient})
	input := storage.ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_context_documents/report.pdf", ETag: `"v1"`, Size: 4}
	metadata, err := reader.Head(context.Background(), input)
	if err != nil || metadata.ETag != input.ETag || metadata.Size != input.Size {
		t.Fatalf("head metadata=%#v err=%v", metadata, err)
	}
	body, metadata, err := reader.Open(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "data" || metadata.MIMEType != "application/pdf" {
		t.Fatalf("body=%q metadata=%#v", data, metadata)
	}
}

func TestConditionalObjectReaderMapsVersionChanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "precondition", http.StatusPreconditionFailed)
	}))
	defer server.Close()
	reader := NewConditionalObjectReader(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "id", SecretKey: "key", Bucket: "bucket", Region: "test", Endpoint: server.URL,
	}}, ObjectStreamerConfig{Enabled: true, Timeout: time.Second, HTTPClient: http.DefaultClient})
	_, err := reader.Head(context.Background(), storage.ConditionalObjectInput{
		StorageProvider: "cos", ObjectKey: "ai_context_documents/report.pdf", ETag: `"v1"`, Size: 4,
	})
	if !errors.Is(err, storage.ErrConditionalObjectVersionChanged) {
		t.Fatalf("err=%v", err)
	}
}
