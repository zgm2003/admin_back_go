package cos

import (
	"context"
	"errors"
	"hash/crc64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestObjectWriterRejectsDisabledAndInvalidConfig(t *testing.T) {
	writer := NewObjectWriter(ObjectWriterConfig{Enabled: false})
	err := writer.Put(context.Background(), PutInput{
		SecretID: "sid", SecretKey: "skey", Bucket: "bucket-123", Region: "ap-guangzhou", Key: "tauri_updater/windows.json", Body: []byte("{}"),
	})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}

	writer = NewObjectWriter(ObjectWriterConfig{Enabled: true})
	err = writer.Put(context.Background(), PutInput{
		SecretID: "sid", SecretKey: "skey", Bucket: "bucket-123", Region: "ap-guangzhou", Body: []byte("{}"),
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestObjectWriterPutsJSONToScopedKey(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = string(body)
		crc := crc64.Update(0, crc64.MakeTable(crc64.ECMA), body)
		w.Header().Set("x-cos-hash-crc64ecma", strconv.FormatUint(crc, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writer := NewObjectWriter(ObjectWriterConfig{
		Enabled: true,
		Timeout: 2 * time.Second,
	})
	err := writer.Put(context.Background(), PutInput{
		SecretID:    "sid",
		SecretKey:   "skey",
		Bucket:      "bucket-123",
		Region:      "ap-guangzhou",
		Key:         "/tauri_updater/windows-x86_64.json",
		Body:        []byte(`{"version":"1.0.7"}`),
		ContentType: "application/json; charset=utf-8",
		Endpoint:    server.URL,
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if gotPath != "/tauri_updater/windows-x86_64.json" {
		t.Fatalf("unexpected object path %q", gotPath)
	}
	if gotContentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", gotContentType)
	}
	if gotBody != `{"version":"1.0.7"}` {
		t.Fatalf("unexpected body %q", gotBody)
	}
}
