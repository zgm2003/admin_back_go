package openaicompat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/shared/apperror"
)

type memoryPreparedFileOpener struct {
	mu        sync.Mutex
	objects   map[string][]byte
	metadata  map[string]infraai.PreparedFileObjectMetadata
	headCalls []infraai.PreparedFileOpenInput
	openCalls []infraai.PreparedFileOpenInput
}

func (opener *memoryPreparedFileOpener) Head(_ context.Context, input infraai.PreparedFileOpenInput) (infraai.PreparedFileObjectMetadata, error) {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.headCalls = append(opener.headCalls, input)
	metadata, ok := opener.metadata[input.ObjectKey]
	if !ok {
		return infraai.PreparedFileObjectMetadata{}, errors.New("missing object")
	}
	return metadata, nil
}

func (opener *memoryPreparedFileOpener) Open(_ context.Context, input infraai.PreparedFileOpenInput) (io.ReadCloser, infraai.PreparedFileObjectMetadata, error) {
	opener.mu.Lock()
	defer opener.mu.Unlock()
	opener.openCalls = append(opener.openCalls, input)
	body, ok := opener.objects[input.ObjectKey]
	if !ok {
		return nil, infraai.PreparedFileObjectMetadata{}, errors.New("missing object")
	}
	return io.NopCloser(bytes.NewReader(body)), opener.metadata[input.ObjectKey], nil
}

func TestFileManifestMaterializesOfficialChatCompletionsPartsAndExactLength(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	opener := testPreparedFileOpener()
	materialized, err := MaterializeFileManifest(context.Background(), manifest, opener)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(materialized.Body)
	if err != nil {
		t.Fatal(err)
	}
	result := <-materialized.Result
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if materialized.ContentLength != int64(len(body)) || result.Metrics.MaterializedRequestBytes != int64(len(body)) {
		t.Fatalf("length=%d body=%d metrics=%#v", materialized.ContentLength, len(body), result.Metrics)
	}
	if len(opener.headCalls) != 0 || len(opener.openCalls) != 2 {
		t.Fatalf("materializer head=%d open=%d", len(opener.headCalls), len(opener.openCalls))
	}
	if strings.Contains(string(body), "file_ref") || strings.Contains(string(body), "object_key") || strings.Contains(string(body), "etag-v1") {
		t.Fatalf("internal file evidence leaked: %s", body)
	}
	for _, want := range []string{
		`"type":"file","file":{"filename":"a.txt","file_data":"data:text/plain;base64,` + base64.StdEncoding.EncodeToString([]byte("one")) + `"}`,
		`"type":"file","file":{"filename":"b.json","file_data":"data:application/json;base64,` + base64.StdEncoding.EncodeToString([]byte("{}")) + `"}`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("materialized body missing %s: %s", want, body)
		}
	}
	if length, err := FileManifestContentLength(manifest); err != nil || length != int64(len(body)) {
		t.Fatalf("exported length=%d err=%v body=%d", length, err, len(body))
	}
}

func TestFileManifestPreflightChecksEveryObjectBeforeDispatch(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	body, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	opener := testPreparedFileOpener()
	client := New(Config{APIProtocol: "chat_completions", FileOpener: opener})
	metrics, err := client.PreflightPreparedChat(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if metrics == nil || len(opener.headCalls) != 2 || len(opener.openCalls) != 0 {
		t.Fatalf("head=%d open=%d", len(opener.headCalls), len(opener.openCalls))
	}

	opener.metadata[manifest.Files[1].ObjectKey] = infraai.PreparedFileObjectMetadata{ETag: `"changed"`, Size: 2, MIMEType: "application/json"}
	if _, err := client.PreflightPreparedChat(context.Background(), body); err == nil {
		t.Fatal("changed ETag passed preflight")
	}
}

func TestFileManifestPreflightClassifiesMetadataDriftAsPermanentObjectError(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	body, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	opener := testPreparedFileOpener()
	metadata := opener.metadata[manifest.Files[0].ObjectKey]
	metadata.MIMEType = "application/octet-stream"
	opener.metadata[manifest.Files[0].ObjectKey] = metadata
	_, err = New(Config{FileOpener: opener}).PreflightPreparedChat(context.Background(), body)
	if !errors.Is(err, storagecos.ErrInvalidObjectMetadata) {
		t.Fatalf("metadata drift error=%v", err)
	}
}

func TestAPIProtocolSnapshotControlsPreparationButNotRecovery(t *testing.T) {
	input := infraai.ChatInput{
		ModelID: "gpt-test",
		Messages: []infraai.Message{{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
			{Kind: infraai.ContentPartText, Text: "read"},
			{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
				Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`,
				Size: 3, MIMEType: "text/plain", Filename: "a.txt",
			}},
		}}},
	}
	if _, err := New(Config{APIProtocol: "chat_completions", FileOpener: testPreparedFileOpener()}).PrepareChat(context.Background(), input); err == nil {
		t.Fatal("Chat Completions prepared a new native file request")
	}
	if _, err := New(Config{APIProtocol: "responses"}).PrepareChat(context.Background(), input); err == nil {
		t.Fatal("file request prepared without an opener")
	}
	prepared, err := New(Config{APIProtocol: "responses", FileOpener: testPreparedFileOpener()}).PrepareChat(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := infraai.ParsePreparedChatFileManifest(prepared)
	if err != nil || manifest.APIProtocol != "responses" {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	if _, err := New(Config{}).PrepareChat(context.Background(), textChatInput("gpt-test", "text")); err != nil {
		t.Fatalf("plain text requires file configuration: %v", err)
	}

	recovery := New(Config{APIProtocol: "chat_completions", FileOpener: testPreparedFileOpener()})
	if _, err := recovery.PreflightPreparedChat(context.Background(), prepared); err != nil {
		t.Fatalf("persisted mode snapshot was overwritten: %v", err)
	}
}

func TestFileManifestPreparationPreservesHistoryAndCurrentFileOrder(t *testing.T) {
	input := infraai.ChatInput{
		ModelID: "gpt-test",
		Messages: []infraai.Message{
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
				{Kind: infraai.ContentPartText, Text: "historical"},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
					Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`,
					Size: 3, MIMEType: "text/plain", Filename: "a.txt",
				}},
			}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
				{Kind: infraai.ContentPartText, Text: "current"},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
					Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/b.json", ETag: `"etag-v2"`,
					Size: 2, MIMEType: "application/json", Filename: "b.json",
				}},
			}},
		},
	}
	prepared, err := New(Config{APIProtocol: "responses", FileOpener: testPreparedFileOpener()}).PrepareChat(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := infraai.ParsePreparedChatFileManifest(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 2 || manifest.Files[0].Filename != "a.txt" || manifest.Files[1].Filename != "b.json" {
		t.Fatalf("manifest file order=%#v", manifest.Files)
	}
	var request struct {
		Input []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(manifest.Request, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Input) != 2 || request.Input[0].Role != "user" || request.Input[1].Role != "user" ||
		len(request.Input[0].Content) != 2 || request.Input[0].Content[0]["type"] != "input_text" || request.Input[0].Content[0]["text"] != "historical" || request.Input[0].Content[1]["ref"] != "file-1" ||
		len(request.Input[1].Content) != 2 || request.Input[1].Content[0]["type"] != "input_text" || request.Input[1].Content[0]["text"] != "current" || request.Input[1].Content[1]["ref"] != "file-2" {
		t.Fatalf("request message order changed: %#v", request.Input)
	}
}

func TestFileManifestDispatchSetsExactHTTPContentLength(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var readErr error
		received, readErr = io.ReadAll(request.Body)
		if readErr != nil {
			t.Errorf("read body: %v", readErr)
			return
		}
		if request.ContentLength != int64(len(received)) {
			t.Errorf("content length=%d body=%d", request.ContentLength, len(received))
		}
		if request.Header.Get("Idempotency-Key") != "attempt-1" {
			t.Errorf("idempotency key=%q", request.Header.Get("Idempotency-Key"))
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(Config{
		BaseURL: server.URL, APIKey: "secret", StreamHTTPClient: server.Client(),
		APIProtocol: "responses", FileOpener: testPreparedFileOpener(),
	})
	preflightMetrics, err := client.PreflightPreparedChat(context.Background(), prepared)
	if err != nil || preflightMetrics == nil {
		t.Fatalf("preflight metrics=%#v err=%v", preflightMetrics, err)
	}
	result, err := client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
		Body: prepared, IdempotencyKey: "attempt-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.FileInputMetrics == nil || result.FileInputMetrics.MaterializedRequestBytes != int64(len(received)) {
		t.Fatalf("result metrics=%#v body=%d", result.FileInputMetrics, len(received))
	}
	opener := client.fileOpener.(*memoryPreparedFileOpener)
	if len(opener.headCalls) != 2 || len(opener.openCalls) != 2 {
		t.Fatalf("full request head=%d open=%d", len(opener.headCalls), len(opener.openCalls))
	}
	if strings.Contains(string(received), "file_ref") || !strings.Contains(string(received), `"type":"file"`) {
		t.Fatalf("unexpected outbound body: %s", received)
	}
}

func TestFileManifestExplicitHTTPRejectionHasStableFilePartError(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, copyErr := io.Copy(io.Discard, request.Body); copyErr != nil {
			t.Errorf("drain request: %v", copyErr)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "file-rejection-1")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(writer, `{"error":{"message":"file content part is not supported"}}`)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, APIKey: "secret", StreamHTTPClient: server.Client(), FileOpener: testPreparedFileOpener()})
	_, err = client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: "attempt-file-rejected"}, nil)
	if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeRejected || infraai.ProviderRequestIDFromError(err) != "file-rejection-1" {
		t.Fatalf("provider error outcome=%q request_id=%q ok=%v err=%v", outcome, infraai.ProviderRequestIDFromError(err), ok, err)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.Code != "ai.provider.file_part_rejected" || appErr.Category != apperror.CategoryDependency ||
		appErr.HTTPStatus != http.StatusBadGateway || appErr.Retry != apperror.Permanent {
		t.Fatalf("stable file rejection error=%#v wrapped=%v", appErr, err)
	}
}

func TestFileManifestHTTPRejectionWritesRedactedOperatorDiagnostics(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"message":"file type rejected for secret","type":"invalid_request_error","code":"unsupported_file","param":"input[0].content[1]"}}`)
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := New(Config{
		BaseURL: server.URL, APIKey: "secret", StreamHTTPClient: server.Client(),
		FileOpener: testPreparedFileOpener(), Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	_, err = client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
		Body: prepared, IdempotencyKey: "attempt-file-diagnostic",
	}, nil)
	if err == nil {
		t.Fatal("provider rejection returned no error")
	}

	logged := logs.String()
	for _, expected := range []string{
		"AI provider request rejected",
		"status_code=400",
		"error_code=unsupported_file",
		"error_type=invalid_request_error",
		"error_param=input[0].content[1]",
		"[redacted]",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("operator log %q does not contain %q", logged, expected)
		}
	}
	if strings.Contains(logged, "secret") {
		t.Fatalf("operator log leaked API key: %q", logged)
	}
}

func TestFileManifestGenericBadRequestKeepsGenericProviderRejection(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"message":"maximum context length exceeded","type":"invalid_request_error","param":"messages"}}`)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, APIKey: "secret", StreamHTTPClient: server.Client(), FileOpener: testPreparedFileOpener()})
	_, err = client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: "attempt-generic-bad-request"}, nil)
	if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeRejected {
		t.Fatalf("provider outcome=%q ok=%v err=%v", outcome, ok, err)
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) && appErr.Code == "ai.provider.file_part_rejected" {
		t.Fatalf("generic rejection was mislabeled as file rejection: %v", err)
	}
}

func TestFileManifestBodyFailureAfterPossibleDispatchIsOutcomeUnknown(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		defer request.Body.Close()
		buffer := make([]byte, 1)
		if _, readErr := request.Body.Read(buffer); readErr != nil {
			return nil, readErr
		}
		return nil, errors.New("connection closed after request body started")
	})
	client := New(Config{
		BaseURL: "https://provider.test", APIKey: "secret", StreamHTTPClient: &http.Client{Transport: transport},
		FileOpener: testPreparedFileOpener(),
	})

	_, err = client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
		Body: prepared, IdempotencyKey: "attempt-body-failure",
	}, nil)
	if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeUnknown {
		t.Fatalf("body failure outcome=%q ok=%v err=%v", outcome, ok, err)
	}
}

func TestFileManifestMaterializationCancellationUnblocksProducer(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	ctx, cancel := context.WithCancel(context.Background())
	materialized, err := MaterializeFileManifest(ctx, manifest, testPreparedFileOpener())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	readDone := make(chan error, 1)
	go func() {
		_, readErr := io.Copy(io.Discard, materialized.Body)
		readDone <- readErr
	}()
	select {
	case readErr := <-readDone:
		if readErr == nil {
			t.Fatal("canceled materialization returned no reader error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled materialization reader blocked")
	}
	select {
	case result := <-materialized.Result:
		if result.Err == nil {
			t.Fatal("canceled producer returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled producer did not finish")
	}
}

func testPreparedFileManifest(t *testing.T) infraai.PreparedChatFileManifest {
	t.Helper()
	request := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}{Model: "gpt-test"}
	request.Messages = append(request.Messages,
		struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		}{Role: "user", Content: []map[string]any{{"type": "file_ref", "ref": "file-1"}}},
		struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		}{Role: "user", Content: []map[string]any{{"type": "text", "text": "now"}, {"type": "file_ref", "ref": "file-2"}}},
	)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return infraai.PreparedChatFileManifest{
		Schema: infraai.PreparedChatSchemaFileManifestV1, FileInputMode: "chat_completions", Request: raw,
		Files: []infraai.PreparedFileRef{
			{Ref: "file-1", ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`, Size: 3, MIMEType: "text/plain", Filename: "a.txt"},
			{Ref: "file-2", ObjectKey: "ai_chat_attachments/b.json", ETag: `"etag-v2"`, Size: 2, MIMEType: "application/json", Filename: "b.json"},
		},
	}
}

func testPreparedFileOpener() *memoryPreparedFileOpener {
	return &memoryPreparedFileOpener{
		objects: map[string][]byte{"ai_chat_attachments/a.txt": []byte("one"), "ai_chat_attachments/b.json": []byte("{}")},
		metadata: map[string]infraai.PreparedFileObjectMetadata{
			"ai_chat_attachments/a.txt":  {ETag: `"etag-v1"`, Size: 3, MIMEType: "text/plain"},
			"ai_chat_attachments/b.json": {ETag: `"etag-v2"`, Size: 2, MIMEType: "application/json"},
		},
	}
}
