package cos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

func TestConditionalStreamRequiresMatchingETag(t *testing.T) {
	var headCalls atomic.Int32
	var getCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-Match") != `"etag-v1"` {
			http.Error(w, "precondition", http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "4")
		switch request.Method {
		case http.MethodHead:
			headCalls.Add(1)
		case http.MethodGet:
			getCalls.Add(1)
			_, _ = io.WriteString(w, "data")
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	streamer := newTestObjectStreamer(server.URL)
	body, metadata, err := streamer.Open(context.Background(), infraai.PreparedFileOpenInput{
		ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"etag-v1"`, Size: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "data" || metadata.ETag != `"etag-v1"` || metadata.Size != 4 || metadata.MIMEType != "application/pdf" {
		t.Fatalf("body=%q metadata=%#v err=%v", got, metadata, err)
	}
	if headCalls.Load() != 0 || getCalls.Load() != 1 {
		t.Fatalf("head=%d get=%d", headCalls.Load(), getCalls.Load())
	}
}

func TestConditionalStreamRejectsGETMetadataDrift(t *testing.T) {
	var getCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			getCalls.Add(1)
		}
		w.Header().Set("ETag", `"etag-v2"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "4")
	}))
	defer server.Close()

	_, _, err := newTestObjectStreamer(server.URL).Open(context.Background(), infraai.PreparedFileOpenInput{
		ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"etag-v1"`, Size: 4,
	})
	if !errors.Is(err, ErrObjectVersionChanged) || getCalls.Load() != 1 {
		t.Fatalf("err=%v get=%d", err, getCalls.Load())
	}
}

func TestConditionalStreamMapsUnavailableObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	streamer := newTestObjectStreamer(server.URL)
	input := infraai.PreparedFileOpenInput{
		ObjectKey: "ai_chat_attachments/missing.pdf", ETag: `"etag-v1"`, Size: 4,
	}
	if _, err := streamer.Head(context.Background(), input); !errors.Is(err, ErrObjectUnavailable) {
		t.Fatalf("head err=%v", err)
	}
	if _, _, err := streamer.Open(context.Background(), input); !errors.Is(err, ErrObjectUnavailable) {
		t.Fatalf("open err=%v", err)
	}
}

func TestConditionalStreamCancellationClosesBody(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("ETag", `"etag-v1"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "4")
		if request.Method == http.MethodGet {
			close(started)
			w.(http.Flusher).Flush()
			<-request.Context().Done()
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	body, _, err := newTestObjectStreamer(server.URL).Open(ctx, infraai.PreparedFileOpenInput{
		ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"etag-v1"`, Size: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	readDone := make(chan error, 1)
	go func() {
		_, readErr := body.Read(make([]byte, 1))
		readDone <- readErr
	}()
	select {
	case readErr := <-readDone:
		if readErr == nil {
			t.Fatal("canceled object body returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled object body did not unblock")
	}
	_ = body.Close()
}

func TestNativeFileStreamSourcesDoNotReadAll(t *testing.T) {
	for _, path := range []string{"object_stream.go", "../../ai/openaicompat/file_manifest.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "io.ReadAll") {
			t.Fatalf("%s must remain streaming", path)
		}
	}
}

func newTestObjectStreamer(endpoint string) *COSObjectStreamer {
	provider := &staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1",
		Region: "ap-test", Endpoint: endpoint,
	}}
	return NewObjectStreamer(provider, ObjectStreamerConfig{
		Enabled: true, Timeout: time.Second, HTTPClient: http.DefaultClient,
	})
}
