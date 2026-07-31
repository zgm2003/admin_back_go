package openaicompat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestResponsesProtocolMaterializesInputFileAndStreamsUsage(t *testing.T) {
	var path string
	var rawBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
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
		RequestProtocol: "responses", FileOpener: testPreparedFileOpener(),
	})
	prepared, err := client.PrepareChat(context.Background(), infraai.ChatInput{
		Content: "读取附件",
		Inputs: map[string]any{
			"model_id":      "gpt-5.6",
			"system_prompt": "只根据附件回答",
			"attachments": []any{
				map[string]any{"type": "image", "url": "https://example.test/image.png"},
				map[string]any{
					"type": "file", "object_key": "ai_chat_attachments/a.txt", "etag": `"etag-v1"`,
					"size": int64(3), "mime_type": "text/plain", "name": "a.txt",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("PrepareChat: %v", err)
	}
	manifest, err := infraai.ParsePreparedChatFileManifest(prepared)
	if err != nil {
		t.Fatalf("ParsePreparedChatFileManifest: %v", err)
	}
	if manifest.RequestProtocol != "responses" {
		t.Fatalf("request protocol=%q", manifest.RequestProtocol)
	}

	sink := &captureSink{}
	result, err := client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{
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
		`"type":"input_image","image_url":"https://example.test/image.png"`,
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
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"weather\",\"arguments\":\"\"}}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":0,\"delta\":\"{\\\"city\\\":\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":0,\"delta\":\"\\\"北京\\\"}\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"total_tokens\":7}}}\n\n")
	}))
	defer server.Close()

	sink := &captureSink{}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", StreamHTTPClient: server.Client(), RequestProtocol: "responses"}).StreamChat(
		context.Background(),
		infraai.ChatInput{
			Content: "北京天气",
			Inputs:  map[string]any{"model_id": "gpt-5.6"},
			Tools: []infraai.ToolDefinition{{
				Name: "weather", Description: "查询天气",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
			}},
		},
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

func TestChatCompletionsProtocolRejectsNativeFilesBeforeDispatch(t *testing.T) {
	client := New(Config{RequestProtocol: "chat_completions", FileOpener: testPreparedFileOpener()})
	_, err := client.PrepareChat(context.Background(), infraai.ChatInput{Content: "read", Inputs: map[string]any{
		"model_id": "gpt-test",
		"attachments": []any{map[string]any{
			"type": "file", "object_key": "ai_chat_attachments/a.txt", "etag": `"etag-v1"`,
			"size": int64(3), "mime_type": "text/plain", "name": "a.txt",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "Responses") {
		t.Fatalf("chat completions file error=%v", err)
	}
}
