package databaseevolution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	storagecos "admin_back_go/internal/infra/storage/cos"
)

func TestVerifyCOSReferencesUsesRangeReadsAndSanitizesFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") != "bytes=0-0" {
			t.Errorf("Range=%q", request.Header.Get("Range"))
		}
		switch request.URL.Path {
		case "/media/ok-200.png":
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("x"))
		case "/media/ok-206.png":
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = writer.Write([]byte("x"))
		case "/media/missing.png":
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`<Error><Code>NoSuchKey</Code><Message>missing?signature=raw-secret</Message></Error>`))
		default:
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`<Error><Code>InternalError</Code><Message>credential=raw-secret</Message></Error>`))
		}
	}))
	defer server.Close()

	reader := storagecos.NewObjectReader(storagecos.ObjectReaderConfig{
		Enabled: true, MaxBytes: 1, HTTPClient: server.Client(),
	})
	input := storagecos.GetInput{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-123", Region: "ap-test", Endpoint: server.URL,
	}
	results := VerifyCOSReferences(context.Background(), reader, input, []string{
		"media/ok-200.png", "media/ok-206.png", "media/missing.png", "media/dependency.png",
	})
	if len(results) != 4 {
		t.Fatalf("results=%+v", results)
	}
	byKey := make(map[string]COSReferenceResult, len(results))
	for _, result := range results {
		byKey[result.Key] = result
	}
	if byKey["media/ok-200.png"].Status != COSReferenceReachable || byKey["media/ok-206.png"].Status != COSReferenceReachable {
		t.Fatalf("success results=%+v", results)
	}
	if byKey["media/missing.png"].Status != COSReferenceNotFound {
		t.Fatalf("missing result=%+v", byKey["media/missing.png"])
	}
	if byKey["media/dependency.png"].Status != COSReferenceDependency || byKey["media/dependency.png"].DependencyClass != "provider" {
		t.Fatalf("dependency result=%+v", byKey["media/dependency.png"])
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-id", "secret-key", "raw-secret", "signature="} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("manifest leaked %q: %s", secret, encoded)
		}
	}
}
