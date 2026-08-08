package cos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestConditionalObjectPreviewVerifiesContextDocumentBeforeSigning(t *testing.T) {
	var headCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead || request.URL.Path != "/ai_context_documents/2026/08/report.md" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("If-Match") != `"v1"` {
			t.Fatalf("If-Match=%q", request.Header.Get("If-Match"))
		}
		headCalls.Add(1)
		writer.Header().Set("ETag", `"v1"`)
		writer.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		writer.Header().Set("Content-Length", "4")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reader := NewConditionalObjectReader(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}}, ObjectStreamerConfig{Enabled: true, Timeout: time.Second, PreviewTTL: 10 * time.Minute, HTTPClient: server.Client()})
	result, err := reader.Preview(context.Background(), storage.ConditionalObjectPreviewInput{
		Object:   storage.ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_context_documents/2026/08/report.md", ETag: `"v1"`, Size: 4},
		MIMEType: "text/markdown",
	})
	if err != nil {
		t.Fatalf("Preview error=%v", err)
	}
	if headCalls.Load() != 1 || result.ExpiresIn != 300 || result.Metadata.MIMEType != "text/markdown" {
		t.Fatalf("head=%d preview=%#v", headCalls.Load(), result)
	}
	if !strings.HasPrefix(result.URL, server.URL+"/ai_context_documents/2026/08/report.md?") || !strings.Contains(result.URL, "q-signature=") {
		t.Fatalf("signed URL=%q", result.URL)
	}
}

func TestConditionalObjectPreviewRejectsUntrustedOrChangedContextDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"v1"`)
		writer.Header().Set("Content-Type", "application/pdf")
		writer.Header().Set("Content-Length", "4")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	reader := NewConditionalObjectReader(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}}, ObjectStreamerConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})

	_, err := reader.Preview(context.Background(), storage.ConditionalObjectPreviewInput{
		Object:   storage.ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"v1"`, Size: 4},
		MIMEType: "application/pdf",
	})
	if !errors.Is(err, storage.ErrInvalidConditionalObjectPreview) {
		t.Fatalf("untrusted key error=%v", err)
	}
	_, err = reader.Preview(context.Background(), storage.ConditionalObjectPreviewInput{
		Object:   storage.ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_context_documents/report.pdf", ETag: `"v1"`, Size: 4},
		MIMEType: "text/plain",
	})
	if !errors.Is(err, storage.ErrConditionalObjectVersionChanged) {
		t.Fatalf("MIME drift error=%v", err)
	}
}

func TestConditionalObjectReaderKeepsConversationAttachmentSourcesReadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"v1"`)
		writer.Header().Set("Content-Type", "text/plain")
		writer.Header().Set("Content-Length", "4")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	reader := NewConditionalObjectReader(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}}, ObjectStreamerConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})
	_, err := reader.Head(context.Background(), storage.ConditionalObjectInput{
		StorageProvider: "cos", ObjectKey: "ai_chat_attachments/2026/08/report.txt", ETag: `"v1"`, Size: 4,
	})
	if err != nil {
		t.Fatalf("conversation attachment source rejected: %v", err)
	}
}
