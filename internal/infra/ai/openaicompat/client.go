package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

const (
	defaultBaseURL           = "https://api.openai.com/v1"
	defaultTimeout           = 30 * time.Second
	defaultStreamIdleTimeout = 60 * time.Second
)

type Config struct {
	BaseURL           string
	APIKey            string
	HTTPClient        *http.Client
	StreamHTTPClient  *http.Client
	Timeout           time.Duration
	StreamIdleTimeout time.Duration
}

type Client struct {
	baseURL           string
	apiKey            string
	httpClient        *http.Client
	streamHTTPClient  *http.Client
	timeout           time.Duration
	streamIdleTimeout time.Duration
}

func New(config Config) *Client {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	streamIdleTimeout := config.StreamIdleTimeout
	if streamIdleTimeout <= 0 {
		streamIdleTimeout = defaultStreamIdleTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	streamHTTPClient := config.StreamHTTPClient
	if streamHTTPClient == nil {
		streamHTTPClient = &http.Client{}
	}
	return &Client{
		baseURL:           strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:            strings.TrimSpace(config.APIKey),
		httpClient:        httpClient,
		streamHTTPClient:  streamHTTPClient,
		timeout:           timeout,
		streamIdleTimeout: streamIdleTimeout,
	}
}

func (c *Client) Capabilities() infraai.CapabilityMetadata {
	return infraai.CapabilityMetadata{
		SupportedUsageIdentities: []infraai.UsageIdentity{
			{Category: infraai.UsageCategoryInput, Unit: "token"},
			{Category: infraai.UsageCategoryOutput, Unit: "token"},
			{Category: infraai.UsageCategoryCacheRead, Unit: "token"},
			{Category: infraai.UsageCategoryCacheWrite, Unit: "token"},
		},
		SafeInputUpperBoundStrategy: infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1,
		SupportsIdempotencyHeader:   true,
	}
}

func (c *Client) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	client := c
	if strings.TrimSpace(input.BaseURL) != "" || strings.TrimSpace(input.APIKey) != "" || input.TimeoutMs > 0 {
		timeout := c.timeout
		if input.TimeoutMs > 0 {
			timeout = time.Duration(input.TimeoutMs) * time.Millisecond
		}
		client = New(Config{
			BaseURL:           nonEmpty(input.BaseURL, c.baseURL),
			APIKey:            nonEmpty(input.APIKey, c.apiKey),
			HTTPClient:        c.httpClient,
			StreamHTTPClient:  c.streamHTTPClient,
			Timeout:           timeout,
			StreamIdleTimeout: c.streamIdleTimeout,
		})
	}

	start := time.Now()
	req, err := client.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.httpClient.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := client.requireSuccess(resp); err != nil {
		return &infraai.TestConnectionResult{OK: false, Status: resp.Status, LatencyMs: latency, Message: err.Error()}, err
	}
	return &infraai.TestConnectionResult{OK: true, Status: resp.Status, LatencyMs: latency, Message: "ok"}, nil
}

func (c *Client) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	prepared, err := c.PrepareChat(ctx, input)
	if err != nil {
		return nil, err
	}
	return c.streamPreparedChat(ctx, infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: input.IdempotencyKey}, sink, false)
}

func (c *Client) PrepareChat(ctx context.Context, input infraai.ChatInput) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(input.Content) == "" {
		if len(inputAttachments(input.Inputs)) == 0 {
			return nil, fmt.Errorf("%w: missing message content", infraai.ErrInvalidConfig)
		}
	}
	model := inputString(input.Inputs, "model_id")
	if model == "" {
		return nil, fmt.Errorf("%w: missing model_id", infraai.ErrInvalidConfig)
	}
	if len(input.ToolOutputs) > 0 && len(input.ToolCalls) == 0 {
		return nil, fmt.Errorf("%w: tool outputs require preceding tool calls", infraai.ErrInvalidConfig)
	}
	body := chatCompletionRequest{
		Model:         model,
		Stream:        true,
		StreamOptions: &chatStreamOptions{IncludeUsage: true},
		Messages:      chatMessages(input),
		Tools:         chatTools(input.Tools),
	}
	if temperature, ok := inputNumber(input.Inputs, "temperature"); ok {
		body.Temperature = &temperature
	}
	if maxTokens, ok := inputInt(input.Inputs, "max_tokens"); ok {
		body.MaxTokens = &maxTokens
	}
	prepared, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAI chat completion request: %w", err)
	}
	return prepared, nil
}

func (c *Client) StreamPreparedChat(ctx context.Context, input infraai.PreparedChatRequest, sink infraai.EventSink) (*infraai.ChatResult, error) {
	return c.streamPreparedChat(ctx, input, sink, true)
}

func (c *Client) streamPreparedChat(ctx context.Context, input infraai.PreparedChatRequest, sink infraai.EventSink, requireKey bool) (*infraai.ChatResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	if len(input.Body) == 0 || !json.Valid(input.Body) {
		return nil, fmt.Errorf("%w: invalid prepared chat request", infraai.ErrInvalidConfig)
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if requireKey && key == "" {
		return nil, fmt.Errorf("%w: missing prepared chat idempotency key", infraai.ErrInvalidConfig)
	}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/chat/completions", input.Body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	streamClient := c.streamHTTPClient
	if streamClient == nil {
		streamClient = &http.Client{}
	}
	streamIdleTimeout := c.streamIdleTimeout
	if streamIdleTimeout <= 0 {
		streamIdleTimeout = defaultStreamIdleTimeout
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "", fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err))
	}
	defer resp.Body.Close()
	providerRequestID := strings.TrimSpace(resp.Header.Get("X-Request-Id"))
	if err := c.requireSuccess(resp); err != nil {
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeRejected, providerRequestID, err)
	}
	watcher := newStreamIdleWatcher(streamIdleTimeout, resp.Body.Close)
	defer watcher.Stop()
	result, err := c.readChatCompletionStream(ctx, resp.Body, sink, func() {
		watcher.Touch(streamIdleTimeout)
	})
	if err != nil {
		if watcher.TimedOut() {
			return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, providerRequestID, fmt.Errorf("%w: OpenAI chat completion stream idle timeout after %s", context.DeadlineExceeded, streamIdleTimeout))
		}
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, providerRequestID, err)
	}
	result.ProviderRequestID = providerRequestID
	result.DispatchState = infraai.DispatchStateDispatched
	return result, nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	var data []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode OpenAI request: %w", err)
		}
		data = encoded
	}
	return c.newJSONRequest(ctx, method, endpoint, data)
}

func (c *Client) newJSONRequest(ctx context.Context, method string, endpoint string, body []byte) (*http.Request, error) {
	baseURL, err := normalizeBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: missing OpenAI API key", infraai.ErrInvalidConfig)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.httpClient == nil {
		timeout := c.timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		c.httpClient = &http.Client{Timeout: timeout}
	}
	return req, nil
}

type streamIdleWatcher struct {
	timer     *time.Timer
	closeBody func() error
	timedOut  atomic.Bool

	mu      sync.Mutex
	stopped bool
}

func newStreamIdleWatcher(timeout time.Duration, closeBody func() error) *streamIdleWatcher {
	if timeout <= 0 {
		timeout = defaultStreamIdleTimeout
	}
	w := &streamIdleWatcher{closeBody: closeBody}
	w.timer = time.AfterFunc(timeout, func() {
		w.timedOut.Store(true)
		if w.closeBody != nil {
			_ = w.closeBody()
		}
	})
	return w
}

func (w *streamIdleWatcher) Touch(timeout time.Duration) {
	if w == nil {
		return
	}
	if timeout <= 0 {
		timeout = defaultStreamIdleTimeout
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.timer.Reset(timeout)
}

func (w *streamIdleWatcher) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	w.timer.Stop()
}

func (w *streamIdleWatcher) TimedOut() bool {
	return w != nil && w.timedOut.Load()
}

func (c *Client) requireSuccess(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("%w: %s", infraai.ErrUpstreamFailed, resp.Status)
	}
	message := upstreamHTTPErrorMessage(body, c.apiKey)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %s %s", infraai.ErrUnauthorized, resp.Status, message)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s %s", infraai.ErrRateLimited, resp.Status, message)
	}
	return fmt.Errorf("%w: %s %s", infraai.ErrUpstreamFailed, resp.Status, message)
}

func (c *Client) readChatCompletionStream(ctx context.Context, body io.Reader, sink infraai.EventSink, touch func()) (*infraai.ChatResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var answer strings.Builder
	result := &infraai.ChatResult{}
	streamDigest := sha256.New()
	hasStreamData := false
	deliveryEnabled := sink != nil
	for scanner.Scan() {
		if touch != nil {
			touch()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		_, _ = io.WriteString(streamDigest, data+"\n")
		hasStreamData = true
		if data == "[DONE]" {
			if result.UsageStatus == "" {
				result.UsageStatus = infraai.UsageStatusUnavailable
				result.Usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
			}
			setStreamResponseHash(result, streamDigest, hasStreamData)
			return result, nil
		}
		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("decode OpenAI chat completion stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			result.PromptTokens = usageInt(chunk.Usage.PromptTokens)
			result.CompletionTokens = usageInt(chunk.Usage.CompletionTokens)
			result.TotalTokens = usageInt(chunk.Usage.TotalTokens)
			result.Usage = tokenUsageSnapshot(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens, chunk.Usage.PromptDetails)
			result.Usage.RawProviderJSON = append([]byte(nil), data...)
			result.Usage.ResponseSHA256 = sha256.Sum256([]byte(data))
			result.ResponseSHA256 = result.Usage.ResponseSHA256
			if result.Usage.Complete() {
				result.UsageStatus = infraai.UsageStatusReported
			} else {
				result.UsageStatus = infraai.UsageStatusUnavailable
			}
		}
		for _, choice := range chunk.Choices {
			for _, call := range choice.Delta.ToolCalls {
				mergeToolCall(result, call)
			}
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			answer.WriteString(delta)
			result.Answer = strings.TrimSpace(answer.String())
			if deliveryEnabled {
				if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
					deliveryEnabled = false
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read OpenAI chat completion stream: %w", err)
	}
	if result.UsageStatus == "" {
		result.UsageStatus = infraai.UsageStatusUnavailable
		result.Usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	setStreamResponseHash(result, streamDigest, hasStreamData)
	return result, nil
}

func setStreamResponseHash(result *infraai.ChatResult, digest hash.Hash, hasData bool) {
	if result == nil || !hasData || digest == nil {
		return
	}
	copy(result.ResponseSHA256[:], digest.Sum(nil))
}

type chatCompletionRequest struct {
	Model         string             `json:"model"`
	Messages      []chatMessage      `json:"messages"`
	Stream        bool               `json:"stream"`
	StreamOptions *chatStreamOptions `json:"stream_options,omitempty"`
	Tools         []chatTool         `json:"tools,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	MaxTokens     *int               `json:"max_tokens,omitempty"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string                `json:"role"`
	Content    any                   `json:"content"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
	ToolCalls  []chatMessageToolCall `json:"tool_calls,omitempty"`
}

type chatMessageToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string               `json:"content"`
			ToolCalls []chatStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     *int                `json:"prompt_tokens"`
		CompletionTokens *int                `json:"completion_tokens"`
		TotalTokens      *int                `json:"total_tokens"`
		PromptDetails    *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
	} `json:"usage"`
}

type promptTokenDetails struct {
	CachedTokens             *int `json:"cached_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

func usageInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func tokenUsageSnapshot(promptValue, completionValue, totalValue *int, details *promptTokenDetails) infraai.UsageSnapshot {
	if promptValue == nil || completionValue == nil || totalValue == nil {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	prompt, completion, total := *promptValue, *completionValue, *totalValue
	if prompt < 0 || completion < 0 || total < 0 || total != prompt+completion {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	read, write := 0, 0
	hasRead, hasWrite := false, false
	if details != nil {
		cached, explicitRead := usageInt(details.CachedTokens), usageInt(details.CacheReadInputTokens)
		if details.CachedTokens != nil && details.CacheReadInputTokens != nil {
			return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
		}
		read = cached + explicitRead
		write = usageInt(details.CacheCreationInputTokens)
		hasRead = details.CachedTokens != nil || details.CacheReadInputTokens != nil
		hasWrite = details.CacheCreationInputTokens != nil
	}
	if read < 0 || write < 0 || read+write > prompt {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	items := []infraai.UsageItem{{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: int64(prompt - read - write)}, {Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: int64(completion)}}
	if details != nil {
		if hasRead {
			items = append(items, infraai.UsageItem{Category: infraai.UsageCategoryCacheRead, Unit: "token", Quantity: int64(read)})
		}
		if hasWrite {
			items = append(items, infraai.UsageItem{Category: infraai.UsageCategoryCacheWrite, Unit: "token", Quantity: int64(write)})
		}
	}
	snapshot, err := infraai.NewUsageSnapshot(infraai.UsageStatusReported, nil, items)
	if err != nil {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	return snapshot
}

type chatStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func chatMessages(input infraai.ChatInput) []chatMessage {
	messages := []chatMessage{}
	if systemPrompt := inputString(input.Inputs, "system_prompt"); systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, historyMessages(input.Inputs)...)
	messages = append(messages, chatMessage{Role: "user", Content: userContent(input)})
	if toolCalls := chatAssistantToolCalls(input.ToolCalls); len(toolCalls) > 0 {
		messages = append(messages, chatMessage{Role: "assistant", Content: nil, ToolCalls: toolCalls})
	}
	for _, output := range input.ToolOutputs {
		if strings.TrimSpace(output.CallID) == "" || strings.TrimSpace(output.Name) == "" {
			continue
		}
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: strings.TrimSpace(output.CallID), Content: strings.TrimSpace(output.Output)})
	}
	return messages
}

func chatAssistantToolCalls(calls []infraai.ToolCall) []chatMessageToolCall {
	out := make([]chatMessageToolCall, 0, len(calls))
	for _, call := range calls {
		id := strings.TrimSpace(call.ID)
		name := strings.TrimSpace(call.Name)
		if id == "" || name == "" {
			continue
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		out = append(out, chatMessageToolCall{
			ID:   id,
			Type: "function",
			Function: chatToolCallFunction{
				Name:      name,
				Arguments: arguments,
			},
		})
	}
	return out
}

func chatTools(tools []infraai.ToolDefinition) []chatTool {
	out := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
		out = append(out, chatTool{Type: "function", Function: chatFunction{Name: name, Description: strings.TrimSpace(tool.Description), Parameters: params}})
	}
	return out
}

func historyMessages(inputs map[string]any) []chatMessage {
	raw := inputs["history"]
	rows, ok := raw.([]map[string]string)
	if !ok {
		return nil
	}
	messages := make([]chatMessage, 0, len(rows))
	for _, row := range rows {
		role := strings.TrimSpace(row["role"])
		content := strings.TrimSpace(row["content"])
		if content == "" {
			continue
		}
		switch role {
		case "user", "assistant", "system":
			messages = append(messages, chatMessage{Role: role, Content: content})
		}
	}
	return messages
}

func userContent(input infraai.ChatInput) any {
	text := strings.TrimSpace(input.Content)
	attachments := inputAttachments(input.Inputs)
	if len(attachments) == 0 {
		return text
	}
	parts := make([]map[string]any, 0, len(attachments)+1)
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, attachment := range attachments {
		parts = append(parts, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": attachment.URL,
			},
		})
	}
	return parts
}

type imageAttachment struct {
	URL string
}

func inputAttachments(inputs map[string]any) []imageAttachment {
	raw, ok := inputs["attachments"].([]any)
	if !ok {
		return nil
	}
	out := make([]imageAttachment, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := row["type"].(string); strings.TrimSpace(typ) != "image" {
			continue
		}
		url, _ := row["url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		out = append(out, imageAttachment{URL: url})
	}
	return out
}

func inputString(inputs map[string]any, key string) string {
	if inputs == nil {
		return ""
	}
	value, _ := inputs[key].(string)
	return strings.TrimSpace(value)
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func inputNumber(inputs map[string]any, key string) (float64, bool) {
	if inputs == nil {
		return 0, false
	}
	switch value := inputs[key].(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		n, err := value.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

func inputInt(inputs map[string]any, key string) (int, bool) {
	number, ok := inputNumber(inputs, key)
	if !ok || number < 1 {
		return 0, false
	}
	return int(number), true
}

func normalizeBaseURL(value string) (string, error) {
	return infraai.NormalizeOpenAIBaseURL(value, defaultBaseURL)
}

func sanitizeBody(body []byte, apiKey string) string {
	compact := strings.TrimSpace(string(body))
	if apiKey != "" {
		compact = strings.ReplaceAll(compact, apiKey, "[redacted]")
	}
	if len(compact) > 512 {
		compact = compact[:512]
	}
	return compact
}

func upstreamHTTPErrorMessage(body []byte, apiKey string) string {
	detail := extractUpstreamErrorDetail(body)
	if detail == "" {
		return sanitizeBody(body, apiKey)
	}
	return sanitizeBody([]byte(detail), apiKey)
}

func extractUpstreamErrorDetail(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if errorValue, ok := payload["error"].(map[string]any); ok {
		if message := stringFromAny(errorValue["message"]); strings.TrimSpace(message) != "" {
			return message
		}
		if detail := stringFromAny(errorValue["detail"]); strings.TrimSpace(detail) != "" {
			return detail
		}
	}
	for _, key := range []string{"message", "msg", "error_message", "detail"} {
		if message := stringFromAny(payload[key]); strings.TrimSpace(message) != "" {
			return message
		}
	}
	return ""
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func mergeToolCall(result *infraai.ChatResult, call chatStreamToolCall) {
	if result == nil {
		return
	}
	idx := call.Index
	for len(result.ToolCalls) <= idx {
		result.ToolCalls = append(result.ToolCalls, infraai.ToolCall{})
	}
	current := result.ToolCalls[idx]
	if strings.TrimSpace(call.ID) != "" {
		current.ID = strings.TrimSpace(call.ID)
	}
	if strings.TrimSpace(call.Function.Name) != "" {
		current.Name = strings.TrimSpace(call.Function.Name)
	}
	if call.Function.Arguments != "" {
		current.Arguments += call.Function.Arguments
	}
	result.ToolCalls[idx] = current
}
