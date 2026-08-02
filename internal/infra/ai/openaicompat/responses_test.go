package openaicompat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestResponsesInlinePreparedRequestFreezesProtocolForRecovery(t *testing.T) {
	prepared, err := New(Config{APIProtocol: infraai.APIProtocolResponses}).PrepareChat(context.Background(), textChatInput("gpt-5.6", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := infraai.DetectPreparedChatSchema(prepared)
	if err != nil || schema != infraai.PreparedChatSchemaResponsesInlineV1 {
		t.Fatalf("prepared schema=%q err=%v body=%s", schema, err, prepared)
	}
	envelope, err := infraai.ParsePreparedChatInlineEnvelope(prepared)
	if err != nil || envelope.APIProtocol != infraai.APIProtocolResponses {
		t.Fatalf("prepared envelope=%+v err=%v", envelope, err)
	}

	var path string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		if decodeErr := json.NewDecoder(request.Body).Decode(&requestBody); decodeErr != nil {
			t.Errorf("decode request: %v", decodeErr)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_inline\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
	}))
	defer server.Close()

	result, err := New(Config{
		BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(),
		APIProtocol: infraai.APIProtocolChatCompletions,
	}).StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
		Body: prepared, IdempotencyKey: "attempt-inline-responses",
	}, nil)
	if err != nil {
		t.Fatalf("recover prepared Responses request: %v", err)
	}
	if path != "/v1/responses" || result.Answer != "ok" {
		t.Fatalf("path=%q result=%+v", path, result)
	}
	if requestBody["schema"] != nil || requestBody["api_protocol"] != nil || requestBody["input"] == nil {
		t.Fatalf("prepared envelope leaked upstream: %#v", requestBody)
	}
}

func TestResponsesProtocolMaterializesInputFileAndStreamsUsage(t *testing.T) {
	var path string
	var rawBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		if request.Header.Get("Idempotency-Key") != "attempt-responses-1" {
			t.Errorf("idempotency key=%q", request.Header.Get("Idempotency-Key"))
		}
		var err error
		rawBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-Id", "provider-request-responses-1")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"文件\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"收到\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"total_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":3}}}}\n\n")
	}))
	defer server.Close()

	client := New(Config{
		BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(),
		APIProtocol: "responses", FileOpener: testPreparedFileOpener(),
	})
	prepared, err := client.PrepareChat(context.Background(), infraai.ChatInput{
		ModelID: "gpt-5.6",
		Messages: []infraai.Message{
			{Role: infraai.MessageRoleSystem, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "只根据附件回答"}}},
			{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
				{Kind: infraai.ContentPartText, Text: "读取附件"},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{Kind: infraai.AttachmentImage, URL: "https://example.test/image.png"}},
				{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
					Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`,
					Size: 3, MIMEType: "text/plain", Filename: "a.txt",
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("PrepareChat: %v", err)
	}
	manifest, err := infraai.ParsePreparedChatFileManifest(prepared)
	if err != nil {
		t.Fatalf("ParsePreparedChatFileManifest: %v", err)
	}
	if manifest.APIProtocol != "responses" {
		t.Fatalf("API protocol=%q", manifest.APIProtocol)
	}

	sink := &captureSink{}
	recoveryClient := New(Config{
		BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(),
		APIProtocol: infraai.APIProtocolChatCompletions, FileOpener: testPreparedFileOpener(),
	})
	result, err := recoveryClient.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
		Body: prepared, IdempotencyKey: "attempt-responses-1",
	}, sink)
	if err != nil {
		t.Fatalf("StreamPreparedChat: %v", err)
	}
	if path != "/v1/responses" {
		t.Fatalf("request path=%q", path)
	}
	if strings.Contains(string(rawBody), "file_ref") || strings.Contains(string(rawBody), "object_key") {
		t.Fatalf("internal file facts leaked: %s", rawBody)
	}
	wantFile := `"type":"input_file","filename":"a.txt","file_data":"data:text/plain;base64,` + base64.StdEncoding.EncodeToString([]byte("one")) + `"`
	for _, want := range []string{
		`"instructions":"只根据附件回答"`,
		`"type":"input_image"`,
		`"image_url":"https://example.test/image.png"`,
		wantFile,
		`"store":false`,
	} {
		if !strings.Contains(string(rawBody), want) {
			t.Fatalf("responses body missing %s: %s", want, rawBody)
		}
	}
	if result.Answer != "文件收到" || result.ProviderRequestID != "provider-request-responses-1" {
		t.Fatalf("result=%+v", result)
	}
	if result.EngineTaskID != "resp_1" || result.ResponseSHA256 == ([32]byte{}) || result.ResponseSHA256 == result.Usage.ResponseSHA256 {
		t.Fatalf("response identity/hash result=%+v usage_hash=%x", result, result.Usage.ResponseSHA256)
	}
	if result.PromptTokens != 10 || result.CompletionTokens != 2 || result.TotalTokens != 12 || result.UsageStatus != infraai.UsageStatusReported {
		t.Fatalf("usage result=%+v snapshot=%+v", result, result.Usage)
	}
	wantUsage := []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 7},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryCacheRead, Unit: "token", Quantity: 3},
	}
	if !reflect.DeepEqual(result.Usage.Items, wantUsage) {
		t.Fatalf("usage items=%+v, want %+v", result.Usage.Items, wantUsage)
	}
	if len(sink.events) != 2 || sink.events[0].DeltaText != "文件" || sink.events[1].DeltaText != "收到" {
		t.Fatalf("sink events=%+v", sink.events)
	}
}

func TestResponsesProtocolMapsFunctionToolsAndStreamedCalls(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_item.added\",\"output_index\":4,\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"weather\",\"arguments\":\"\"}}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":4,\"delta\":\"{\\\"city\\\":\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":4,\"delta\":\"\\\"北京\\\"}\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_1\",\"output_index\":4,\"arguments\":\"{\\\"city\\\":\\\"北京\\\"}\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_item.done\",\"output_index\":4,\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"weather\",\"arguments\":\"{\\\"city\\\":\\\"北京\\\"}\"}}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"total_tokens\":7}}}\n\n")
	}))
	defer server.Close()

	sink := &captureSink{}
	input := textChatInput("gpt-5.6", "北京天气")
	input.Tools = []infraai.ToolDefinition{{
		Name: "weather", Description: "查询天气",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
	}}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(), APIProtocol: "responses"}).StreamChat(
		context.Background(),
		input,
		sink,
	)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools=%#v", requestBody["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "weather" || tool["function"] != nil {
		t.Fatalf("responses function tool=%#v", tool)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call_1" || result.ToolCalls[0].Name != "weather" || result.ToolCalls[0].Arguments != `{"city":"北京"}` {
		t.Fatalf("tool calls=%+v", result.ToolCalls)
	}
	if len(sink.events) != 3 {
		t.Fatalf("tool events=%+v", sink.events)
	}
}

func TestResponsesProtocolPreservesReasoningContinuationForToolOutput(t *testing.T) {
	firstStream := strings.Join([]string{
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","encrypted_content":"opaque-reasoning"}}`,
		`data: {"type":"response.output_item.added","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"{\"city\":\"北京\"}"}`,
		`data: {"type":"response.output_item.done","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"北京\"}"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_tool","status":"completed","output":[{"id":"rs_1","type":"reasoning","encrypted_content":"opaque-reasoning"},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"北京\"}"}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}}`,
	}, "\n\n") + "\n\n"
	client := New(Config{APIProtocol: infraai.APIProtocolResponses})
	result, err := client.readResponsesStream(context.Background(), strings.NewReader(firstStream), nil, nil)
	if err != nil {
		t.Fatalf("read first Responses turn: %v", err)
	}
	if result.Continuation == nil || result.Continuation.Protocol != infraai.APIProtocolResponses ||
		!strings.Contains(string(result.Continuation.Items), `"encrypted_content":"opaque-reasoning"`) {
		t.Fatalf("continuation=%#v", result.Continuation)
	}

	input := textChatInput("gpt-5.6", "北京天气")
	input.ToolCalls = result.ToolCalls
	input.Continuation = result.Continuation
	input.ToolOutputs = []infraai.ToolOutput{{CallID: "call_1", Name: "weather", Output: `{"temperature":26}`}}
	prepared, err := client.PrepareChat(context.Background(), input)
	if err != nil {
		t.Fatalf("prepare continuation request: %v", err)
	}
	envelope, err := infraai.ParsePreparedChatInlineEnvelope(prepared)
	if err != nil {
		t.Fatalf("decode continuation envelope: %v", err)
	}
	var request struct {
		Input []json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(envelope.Request, &request); err != nil {
		t.Fatalf("decode continuation request: %v", err)
	}
	joined := string(bytesJoin(request.Input))
	if strings.Count(joined, `"type":"function_call"`) != 1 ||
		!strings.Contains(joined, `"encrypted_content":"opaque-reasoning"`) ||
		!strings.Contains(joined, `"type":"function_call_output","call_id":"call_1","output":"{\"temperature\":26}"`) {
		t.Fatalf("continuation input=%s", joined)
	}
}

func TestResponsesUsageMapsCacheWriteTokens(t *testing.T) {
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_usage\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"total_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":3,\"cache_write_tokens\":2}}}}\n\n"
	result, err := New(Config{APIProtocol: infraai.APIProtocolResponses}).readResponsesStream(context.Background(), strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 5},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryCacheRead, Unit: "token", Quantity: 3},
		{Category: infraai.UsageCategoryCacheWrite, Unit: "token", Quantity: 2},
	}
	if !reflect.DeepEqual(result.Usage.Items, want) {
		t.Fatalf("usage items=%+v, want %+v", result.Usage.Items, want)
	}
}

func TestResponsesCompletedUsesAuthoritativeFinalOutputText(t *testing.T) {
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_text\",\"status\":\"completed\",\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"final answer\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":2,\"total_tokens\":4}}}\n\n"
	result, err := New(Config{APIProtocol: infraai.APIProtocolResponses}).readResponsesStream(context.Background(), strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "final answer" || result.EngineTaskID != "resp_text" {
		t.Fatalf("result=%+v", result)
	}
}

func TestResponsesTerminalFailuresReturnDispatchEvidence(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		completeUsage bool
	}{
		{
			name:          "failed",
			data:          `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"server_error","message":"failed"},"usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5}}}`,
			completeUsage: true,
		},
		{
			name: "error",
			data: `{"type":"error","code":"server_error","message":"stream failed"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.Header().Set("X-Request-Id", "request-"+test.name)
				_, _ = fmt.Fprintf(writer, "data: %s\n\n", test.data)
			}))
			defer server.Close()

			result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(), APIProtocol: infraai.APIProtocolResponses}).StreamChat(
				context.Background(), textChatInput("gpt-5.6", "hello"), nil,
			)
			if err == nil {
				t.Fatal("terminal failure returned no error")
			}
			if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeRejected {
				t.Fatalf("outcome=%q ok=%v err=%v", outcome, ok, err)
			}
			if result == nil || result.ProviderRequestID != "request-"+test.name || result.DispatchState != infraai.DispatchStateDispatched || result.ResponseSHA256 == ([32]byte{}) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if test.completeUsage != result.Usage.Complete() {
				t.Fatalf("usage=%+v complete=%v", result.Usage, result.Usage.Complete())
			}
			if test.completeUsage && (len(result.Usage.RawProviderJSON) == 0 || sha256.Sum256(result.Usage.RawProviderJSON) != result.Usage.ResponseSHA256) {
				t.Fatalf("terminal usage evidence=%+v", result.Usage)
			}
		})
	}
}

func TestResponsesFailedWritesRedactedOperatorDiagnostics(t *testing.T) {
	const apiKey = "sk-sensitive"
	stream := `data: {"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"unsupported_file","message":"file rejected for sk-sensitive"}}}` + "\n\n"
	var logs bytes.Buffer
	client := New(Config{
		APIKey: apiKey,
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})

	_, err := client.readResponsesStream(context.Background(), strings.NewReader(stream), nil, nil)
	if err == nil {
		t.Fatal("response.failed returned no error")
	}
	logged := logs.String()
	for _, expected := range []string{
		"AI provider stream failed",
		"event_type=response.failed",
		"error_code=unsupported_file",
		"[redacted]",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("operator log %q does not contain %q", logged, expected)
		}
	}
	if strings.Contains(logged, apiKey) {
		t.Fatalf("operator log leaked API key: %q", logged)
	}
}

func TestResponsesIncompleteReturnsPartialResultAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-Id", "request-incomplete")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial answer\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp_incomplete\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":4,\"output_tokens\":2,\"total_tokens\":6}}}\n\n")
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(), APIProtocol: infraai.APIProtocolResponses}).StreamChat(
		context.Background(), textChatInput("gpt-5.6", "hello"), nil,
	)
	if err != nil {
		t.Fatalf("incomplete response returned error: %v", err)
	}
	if result == nil || result.Answer != "partial answer" || result.EngineTaskID != "resp_incomplete" ||
		result.ProviderRequestID != "request-incomplete" || result.DispatchState != infraai.DispatchStateDispatched ||
		!result.Usage.Complete() || result.PromptTokens != 4 || result.CompletionTokens != 2 || result.TotalTokens != 6 {
		t.Fatalf("incomplete result=%+v", result)
	}
}

func TestResponsesEOFWithoutTerminalIsOutcomeUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	}))
	defer server.Close()
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(), APIProtocol: infraai.APIProtocolResponses}).StreamChat(
		context.Background(), textChatInput("gpt-5.6", "hello"), nil,
	)
	if err == nil || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeUnknown {
		t.Fatalf("outcome=%q ok=%v err=%v", outcome, ok, err)
	}
}

func TestLegacyChatFileManifestRecoveryIgnoresCurrentResponsesProtocol(t *testing.T) {
	manifest := testPreparedFileManifest(t)
	prepared, err := infraai.MarshalPreparedChatFileManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	_, err = New(Config{
		BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(),
		APIProtocol: infraai.APIProtocolResponses, FileOpener: testPreparedFileOpener(),
	}).StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: "legacy-attempt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("legacy recovery path=%q", path)
	}
}

func TestPreparedTextResponsesRecoveryIgnoresCurrentChatProtocol(t *testing.T) {
	prepared, err := New(Config{APIProtocol: infraai.APIProtocolResponses}).PrepareChat(context.Background(), textChatInput("gpt-5.6", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
	}))
	defer server.Close()
	result, err := New(Config{
		BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(),
		APIProtocol: infraai.APIProtocolChatCompletions,
	}).StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: "responses-text-attempt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/responses" || result.EngineTaskID != "resp_recovered" {
		t.Fatalf("path=%q result=%+v", path, result)
	}
}

func bytesJoin(values []json.RawMessage) []byte {
	var joined []byte
	for _, value := range values {
		joined = append(joined, value...)
	}
	return joined
}

func TestChatCompletionsProtocolRejectsNativeFilesBeforeDispatch(t *testing.T) {
	client := New(Config{APIProtocol: "chat_completions", FileOpener: testPreparedFileOpener()})
	_, err := client.PrepareChat(context.Background(), infraai.ChatInput{
		ModelID: "gpt-test",
		Messages: []infraai.Message{{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{
			{Kind: infraai.ContentPartText, Text: "read"},
			{Kind: infraai.ContentPartAttachment, Attachment: &infraai.AttachmentRef{
				Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/a.txt", ETag: `"etag-v1"`,
				Size: 3, MIMEType: "text/plain", Filename: "a.txt",
			}},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "Responses") {
		t.Fatalf("chat completions file error=%v", err)
	}
}
