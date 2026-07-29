package openaicompat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stream disconnected") }

type captureSink struct {
	events []infraai.Event
}

type failingDeliverySink struct {
	calls int
	err   error
}

func TestCompatibleClientDoesNotClaimTokenizerUpperBoundCapability(t *testing.T) {
	capabilities := New(Config{}).Capabilities()
	if capabilities.SafeInputUpperBoundStrategy != infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1 {
		t.Fatalf("compatible transport strategy=%q", capabilities.SafeInputUpperBoundStrategy)
	}
	wantUsage := []infraai.UsageIdentity{
		{Category: infraai.UsageCategoryInput, Unit: "token"},
		{Category: infraai.UsageCategoryOutput, Unit: "token"},
		{Category: infraai.UsageCategoryCacheRead, Unit: "token"},
		{Category: infraai.UsageCategoryCacheWrite, Unit: "token"},
	}
	if !reflect.DeepEqual(capabilities.SupportedUsageIdentities, wantUsage) {
		t.Fatalf("supported usage identities=%+v, want %+v", capabilities.SupportedUsageIdentities, wantUsage)
	}
	if !reflect.DeepEqual(capabilities.InputModalities, []string{"text", "image"}) ||
		!reflect.DeepEqual(capabilities.OutputModalities, []string{"text"}) ||
		!reflect.DeepEqual(capabilities.SupportedParameters, []string{"temperature"}) {
		t.Fatalf("compatible transport capability sets=%+v", capabilities)
	}
	if !capabilities.SupportsTools || !capabilities.SupportsStreaming || !capabilities.SupportsStructuredOutput {
		t.Fatalf("compatible transport capability flags=%+v", capabilities)
	}
}

func (s *captureSink) Emit(ctx context.Context, event infraai.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *failingDeliverySink) Emit(context.Context, infraai.Event) error {
	s.calls++
	return s.err
}

func TestClientStreamChatParsesSSEChunksAndEmitsEveryDelta(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	sink := &captureSink{}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-5.4"},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if requestBody["stream"] != true {
		t.Fatalf("stream = %#v, want true", requestBody["stream"])
	}
	streamOptions, ok := requestBody["stream_options"].(map[string]any)
	if !ok || streamOptions["include_usage"] != true {
		t.Fatalf("expected stream_options.include_usage=true, got %#v", requestBody["stream_options"])
	}
	if result.Answer != "你好" || result.PromptTokens != 2 || result.CompletionTokens != 2 || result.TotalTokens != 4 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(sink.events) != 2 || sink.events[0].DeltaText != "你" || sink.events[1].DeltaText != "好" {
		t.Fatalf("unexpected sink events: %#v", sink.events)
	}
}

func TestClientStreamChatDrainsUsageAfterSinkDeliveryFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "provider-request-drained")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	deliveryErr := errors.New("websocket delivery failed")
	sink := &failingDeliverySink{err: deliveryErr}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-5.4"},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned sink delivery error instead of draining: %v", err)
	}
	if sink.calls != 1 {
		t.Fatalf("sink calls=%d, want only the first failed delivery", sink.calls)
	}
	if result.Answer != "你好" || result.ProviderRequestID != "provider-request-drained" || result.DispatchState != infraai.DispatchStateDispatched {
		t.Fatalf("drained result=%+v", result)
	}
	if result.PromptTokens != 2 || result.CompletionTokens != 2 || result.TotalTokens != 4 || result.UsageStatus != infraai.UsageStatusReported || !result.Usage.Complete() {
		t.Fatalf("drained usage=%+v result=%+v", result.Usage, result)
	}
	if len(result.Usage.RawProviderJSON) == 0 || sha256.Sum256(result.Usage.RawProviderJSON) != result.Usage.ResponseSHA256 || result.ResponseSHA256 == ([32]byte{}) {
		t.Fatalf("missing drained provider evidence: usage=%+v response_hash=%x", result.Usage, result.ResponseSHA256)
	}
}

func TestClientStreamChatMarksMalformedUsageUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":99}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UsageStatus != infraai.UsageStatusUnavailable || result.Usage.Complete() {
		t.Fatalf("malformed usage was accepted: status=%q usage=%+v", result.UsageStatus, result.Usage)
	}
}

func TestClientStreamChatDoesNotTreatOmittedUsageCountsAsZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UsageStatus != infraai.UsageStatusUnavailable || result.Usage.Complete() {
		t.Fatalf("omitted usage was accepted as zero: status=%q usage=%+v", result.UsageStatus, result.Usage)
	}
}

func TestClientStreamChatParsesDirectCacheUsageFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1,\"total_tokens\":11,\"cache_read_input_tokens\":2,\"cache_creation_input_tokens\":3,\"cache_creation\":{\"ephemeral_5m_input_tokens\":1,\"ephemeral_1h_input_tokens\":2}}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "claude-test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UsageStatus != infraai.UsageStatusReported {
		t.Fatalf("direct cache usage unavailable: %+v", result.Usage)
	}
	want := []infraai.UsageItem{
		{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: 5},
		{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: 1},
		{Category: infraai.UsageCategoryCacheRead, Unit: "token", Quantity: 2},
		{Category: infraai.UsageCategoryCacheWrite, Unit: "token", TierKey: "5m", Quantity: 1},
		{Category: infraai.UsageCategoryCacheWrite, Unit: "token", TierKey: "1h", Quantity: 2},
	}
	if !reflect.DeepEqual(result.Usage.Items, want) {
		t.Fatalf("direct cache items=%+v, want %+v", result.Usage.Items, want)
	}
}

func TestClientStreamChatAcceptsConsistentDirectAndPromptCacheVariants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1,\"total_tokens\":11,\"cache_read_input_tokens\":2,\"cache_creation_input_tokens\":3,\"cache_creation\":{\"ephemeral_5m_input_tokens\":1,\"ephemeral_1h_input_tokens\":2},\"prompt_tokens_details\":{\"cache_read_input_tokens\":2,\"cache_creation_input_tokens\":3,\"cache_creation\":{\"ephemeral_5m_input_tokens\":1,\"ephemeral_1h_input_tokens\":2}}}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "claude-test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UsageStatus != infraai.UsageStatusReported || len(result.Usage.Items) != 5 {
		t.Fatalf("consistent cache variants rejected: %+v", result.Usage)
	}
}

func TestClientStreamChatKeepsUntieredDirectCacheCreationTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1,\"total_tokens\":5,\"cache_creation_input_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "claude-test"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Usage.Items {
		if item.Category == infraai.UsageCategoryCacheWrite && item.Quantity == 3 && item.TierKey == "" {
			return
		}
	}
	t.Fatalf("untiered cache creation total missing: %+v", result.Usage)
}

func TestClientStreamChatRejectsDuplicateAndConflictingUsageFields(t *testing.T) {
	tests := map[string]string{
		"duplicate usage field":                 `{"choices":[],"usage":{"prompt_tokens":10,"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`,
		"duplicate cache creation total":        `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"cache_creation_input_tokens":1,"cache_creation_input_tokens":1}}`,
		"duplicate cache creation detail":       `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"cache_creation_input_tokens":2,"cache_creation":{"ephemeral_5m_input_tokens":1,"ephemeral_5m_input_tokens":1}}}`,
		"conflicting direct and prompt details": `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"cache_read_input_tokens":2,"prompt_tokens_details":{"cache_read_input_tokens":3}}}`,
		"conflicting cache creation variants":   `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"cache_creation":{"ephemeral_5m_input_tokens":1},"prompt_tokens_details":{"cache_creation":{"ephemeral_5m_input_tokens":2}}}}`,
	}
	for name, rawChunk := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", rawChunk)
			}))
			defer server.Close()
			result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "claude-test"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.UsageStatus != infraai.UsageStatusUnavailable || result.Usage.Complete() {
				t.Fatalf("invalid raw usage accepted: %+v", result.Usage)
			}
		})
	}
}

func TestClientStreamChatSendsOpenAIChatCompletionAndEmitsDelta(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好，我是测试助手\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	sink := &captureSink{}
	client := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second})
	result, err := client.StreamChat(context.Background(), infraai.ChatInput{
		Content: "你是谁",
		Inputs: map[string]any{
			"model_id":      "gpt-5.4",
			"system_prompt": "你是一个后台助手",
			"history": []map[string]string{
				{"role": "user", "content": "上一轮用户"},
				{"role": "assistant", "content": "上一轮助手"},
			},
		},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "你好，我是测试助手" || result.PromptTokens != 2 || result.CompletionTokens != 3 || result.TotalTokens != 5 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(sink.events) != 1 || sink.events[0].Type != "delta" || sink.events[0].DeltaText != "你好，我是测试助手" {
		t.Fatalf("unexpected sink events: %#v", sink.events)
	}
	if requestBody["model"] != "gpt-5.4" || requestBody["stream"] != true {
		t.Fatalf("unexpected model/stream: %#v", requestBody)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 4 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}
	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, want := range wantRoles {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != want {
			t.Fatalf("message[%d] = %#v, want role %s", i, messages[i], want)
		}
	}
}

func TestClientStreamChatSendsProviderAttemptIdempotencyKey(t *testing.T) {
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "provider-request-7")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test"}).StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}, AttemptID: 9, IdempotencyKey: "attempt-key-9",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if key != "attempt-key-9" || result.ProviderRequestID != "provider-request-7" {
		t.Fatalf("key=%q provider_request_id=%q", key, result.ProviderRequestID)
	}
}

func TestClientPreparedChatDispatchesPersistedBytesAndKeyVerbatim(t *testing.T) {
	var rawBody []byte
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ = io.ReadAll(r.Body)
		gotKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second})
	prepared, err := client.PrepareChat(context.Background(), infraai.ChatInput{Content: "hello", Inputs: map[string]any{"model_id": "gpt-test"}})
	if err != nil {
		t.Fatalf("PrepareChat: %v", err)
	}
	// The persisted bytes are the source of truth; dispatch must not rebuild
	// them from a second ChatInput.
	persisted := append([]byte(nil), prepared...)
	result, err := client.StreamPreparedChat(context.Background(), infraai.PreparedChatRequest{Body: persisted, IdempotencyKey: "attempt-key-11"}, nil)
	if err != nil {
		t.Fatalf("StreamPreparedChat: %v", err)
	}
	if result == nil || result.Answer != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !bytes.Equal(rawBody, persisted) {
		t.Fatalf("provider body changed: got %q want %q", rawBody, persisted)
	}
	if gotKey != "attempt-key-11" {
		t.Fatalf("idempotency key=%q", gotKey)
	}
}

func TestClientStreamChatClassifiesPreHeaderAndPostHeaderFailures(t *testing.T) {
	preHeader := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial refused")
	})}
	_, err := New(Config{BaseURL: "https://provider.test", APIKey: "sk-test", StreamHTTPClient: preHeader}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	if outcome, ok := infraai.ProviderOutcomeFromError(err); !ok || outcome != infraai.ProviderOutcomeUnknown {
		t.Fatalf("pre-header outcome=%q ok=%v err=%v", outcome, ok, err)
	}

	postHeader := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"X-Request-Id": []string{"provider-request-8"}},
			Body:       io.NopCloser(failingReader{}),
		}, nil
	})}
	_, err = New(Config{BaseURL: "https://provider.test", APIKey: "sk-test", StreamHTTPClient: postHeader}).StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	outcome, ok := infraai.ProviderOutcomeFromError(err)
	if !ok || outcome != infraai.ProviderOutcomeUnknown || infraai.ProviderRequestIDFromError(err) != "provider-request-8" {
		t.Fatalf("post-header outcome=%q request_id=%q ok=%v err=%v", outcome, infraai.ProviderRequestIDFromError(err), ok, err)
	}
}

func TestTokenUsageRejectsUndocumentedCacheAggregateAndSubset(t *testing.T) {
	prompt, completion, total := 10, 1, 11
	cached, explicitRead := 0, 2
	usage := tokenUsageSnapshot(&prompt, &completion, &total, &promptTokenDetails{CachedTokens: &cached, CacheReadInputTokens: &explicitRead})
	if usage.Status != infraai.UsageStatusUnavailable {
		t.Fatalf("undocumented cache aggregate/subset relation was accepted: %+v", usage)
	}
}

func TestTokenUsageSplitsCacheCreationDurations(t *testing.T) {
	prompt, completion, total := 12, 1, 13
	fiveMinutes, oneHour := 2, 3
	usage := tokenUsageSnapshot(&prompt, &completion, &total, &promptTokenDetails{
		CacheCreation: &cacheCreationDetails{Ephemeral5mInputTokens: &fiveMinutes, Ephemeral1hInputTokens: &oneHour},
	})
	if usage.Status != infraai.UsageStatusReported {
		t.Fatalf("usage unavailable: %+v", usage)
	}
	var tiers = map[string]int64{}
	for _, item := range usage.Items {
		if item.Category == infraai.UsageCategoryCacheWrite {
			tiers[item.TierKey] = item.Quantity
		}
	}
	if tiers["5m"] != 2 || tiers["1h"] != 3 || len(tiers) != 2 {
		t.Fatalf("cache creation tiers = %#v", tiers)
	}
}

func TestTokenUsageRejectsConflictingCacheCreationDetails(t *testing.T) {
	prompt, completion, total := 10, 1, 11
	for _, values := range [][3]int{{5, 2, 2}, {5, -1, 6}, {11, 5, 6}} {
		write, fiveMinutes, oneHour := values[0], values[1], values[2]
		usage := tokenUsageSnapshot(&prompt, &completion, &total, &promptTokenDetails{
			CacheCreationInputTokens: &write,
			CacheCreation:            &cacheCreationDetails{Ephemeral5mInputTokens: &fiveMinutes, Ephemeral1hInputTokens: &oneHour},
		})
		if usage.Status != infraai.UsageStatusUnavailable {
			t.Fatalf("invalid cache creation values %v accepted: %+v", values, usage)
		}
	}
}

func TestClientStreamChatDoesNotSendSystemMessageWhenSystemPromptBlank(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content: "你是谁",
		Inputs: map[string]any{
			"model_id":      "gpt-5.4",
			"system_prompt": "   ",
		},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["role"] != "user" {
		t.Fatalf("blank system prompt must not produce system message: %#v", messages)
	}
}

func TestOpenAIAdapterUsesSystemEffectiveMaxOutputTokens(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"看到了\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content:                  "看图",
		EffectiveMaxOutputTokens: 2048,
		Inputs: map[string]any{
			"model_id":    "gpt-5.4",
			"temperature": 0.7,
			"max_tokens":  1024.0,
			"attachments": []any{map[string]any{"type": "image", "url": "https://example.test/a.png"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if requestBody["temperature"] != 0.7 || requestBody["max_tokens"] != 2048.0 {
		t.Fatalf("adapter did not use the system output bound: %#v", requestBody)
	}
	messages := requestBody["messages"].([]any)
	userMessage := messages[len(messages)-1].(map[string]any)
	parts, ok := userMessage["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("vision content not sent: %#v", userMessage["content"])
	}
	if parts[0].(map[string]any)["type"] != "text" || parts[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("unexpected content parts: %#v", parts)
	}
}

func TestClientDoesNotLeakAPIKeyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key sk-secret-value"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL, APIKey: "sk-secret-value", Timeout: time.Second}).
		StreamChat(context.Background(), infraai.ChatInput{Content: "hi", Inputs: map[string]any{"model_id": "gpt-test"}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-value") {
		t.Fatalf("error leaked api key: %v", err)
	}
}

func TestClientStreamChatDoesNotUseTotalHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hello"}}]}`)
		flusher.Flush()
		time.Sleep(120 * time.Millisecond)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" world"}}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
		flusher.Flush()
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:           server.URL,
		APIKey:            "sk-test",
		Timeout:           50 * time.Millisecond,
		StreamIdleTimeout: 500 * time.Millisecond,
	})
	result, err := client.StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-test"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "hello world" {
		t.Fatalf("unexpected answer %q", result.Answer)
	}
}

func TestClientStreamChatReturnsIdleTimeoutWhenStreamIsSilent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:           server.URL,
		APIKey:            "sk-test",
		Timeout:           time.Second,
		StreamIdleTimeout: 50 * time.Millisecond,
	})
	_, err := client.StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-test"},
	}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestClientStreamChatSendsToolsAndReturnsToolCalls(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"admin_user_count","arguments":"{"}}]}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	sink := &captureSink{}
	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content: "查用户量",
		Inputs:  map[string]any{"model_id": "gpt-5.4"},
		Tools:   []infraai.ToolDefinition{{Name: "admin_user_count", Description: "查询当前用户量", Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools not sent: %#v", requestBody["tools"])
	}
	tool := tools[0].(map[string]any)
	fn := tool["function"].(map[string]any)
	if tool["type"] != "function" || fn["name"] != "admin_user_count" {
		t.Fatalf("unexpected tool definition: %#v", tool)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call_1" || result.ToolCalls[0].Name != "admin_user_count" || result.ToolCalls[0].Arguments != "{}" {
		t.Fatalf("unexpected tool calls: %#v", result.ToolCalls)
	}
	if result.Answer != "" {
		t.Fatalf("tool-call round must not fake final answer, got %q", result.Answer)
	}
	if len(sink.events) != 2 || sink.events[0].Type != "tool_delta" || sink.events[1].Type != "tool_delta" {
		t.Fatalf("tool deltas=%#v", sink.events)
	}
	if sink.events[0].Payload["tool_call_id"] != "call_1" || sink.events[0].Payload["name"] != "admin_user_count" || sink.events[0].Payload["arguments_delta"] != "{" {
		t.Fatalf("first tool delta=%#v", sink.events[0])
	}
	if sink.events[1].Payload["arguments_delta"] != "}" {
		t.Fatalf("second tool delta=%#v", sink.events[1])
	}
}

func TestClientStreamChatSendsToolOutputsAsToolMessages(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"当前用户量1015\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	result, err := New(Config{BaseURL: server.URL, APIKey: "sk-test", Timeout: time.Second}).StreamChat(context.Background(), infraai.ChatInput{
		Content:     "查用户量",
		Inputs:      map[string]any{"model_id": "gpt-5.4"},
		ToolCalls:   []infraai.ToolCall{{ID: "call_1", Name: "admin_user_count", Arguments: "{}"}},
		ToolOutputs: []infraai.ToolOutput{{CallID: "call_1", Name: "admin_user_count", Output: `{"total_users":1015}`}},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "当前用户量1015" {
		t.Fatalf("unexpected answer: %#v", result)
	}
	messages := requestBody["messages"].([]any)
	if len(messages) < 3 {
		t.Fatalf("tool output request must include user, assistant tool_call, and tool message: %#v", messages)
	}
	assistant := messages[len(messages)-2].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("tool output must be preceded by assistant tool_calls message: %#v", assistant)
	}
	calls, ok := assistant["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("assistant tool_calls missing: %#v", assistant)
	}
	call := calls[0].(map[string]any)
	fn := call["function"].(map[string]any)
	if call["id"] != "call_1" || call["type"] != "function" || fn["name"] != "admin_user_count" || fn["arguments"] != "{}" {
		t.Fatalf("assistant tool_call mismatch: %#v", call)
	}
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "call_1" || last["content"] != `{"total_users":1015}` {
		t.Fatalf("tool output message not sent: %#v", last)
	}
	if _, ok := last["name"]; ok {
		t.Fatalf("Chat Completions tool message must not use legacy name field: %#v", last)
	}
}
