package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
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
	req, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
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
		return nil, fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := c.requireSuccess(resp); err != nil {
		return nil, err
	}
	watcher := newStreamIdleWatcher(streamIdleTimeout, resp.Body.Close)
	defer watcher.Stop()
	result, err := c.readChatCompletionStream(ctx, resp.Body, sink, func() {
		watcher.Touch(streamIdleTimeout)
	})
	if err != nil {
		if watcher.TimedOut() {
			return nil, fmt.Errorf("%w: OpenAI chat completion stream idle timeout after %s", context.DeadlineExceeded, streamIdleTimeout)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateVideo(ctx context.Context, input infraai.VideoInput) (*infraai.VideoTask, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return nil, fmt.Errorf("%w: missing video model", infraai.ErrInvalidConfig)
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("%w: missing video prompt", infraai.ErrInvalidConfig)
	}
	body := map[string]any{"model": model, "prompt": prompt}
	if input.DurationSeconds > 0 {
		body["seconds"] = input.DurationSeconds
	}
	if size := strings.TrimSpace(input.Size); size != "" {
		body["size"] = size
	}
	if resolution := strings.TrimSpace(input.ResolutionName); resolution != "" {
		body["resolution_name"] = resolution
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/videos", body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := c.requireSuccess(resp); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OpenAI video create response: %w", err)
	}
	task := videoTaskFromPayload(payload)
	if strings.TrimSpace(task.ID) == "" {
		return nil, fmt.Errorf("%w: video create response missing task id", infraai.ErrUpstreamFailed)
	}
	return task, nil
}

func (c *Client) GetVideo(ctx context.Context, taskID string) (*infraai.VideoTask, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: missing video task id", infraai.ErrInvalidConfig)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/videos/"+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := c.requireSuccess(resp); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OpenAI video status response: %w", err)
	}
	task := videoTaskFromPayload(payload)
	if task.ID == "" {
		task.ID = taskID
	}
	return task, nil
}

func (c *Client) DownloadVideo(ctx context.Context, taskID string) ([]byte, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, "", fmt.Errorf("%w: missing video task id", infraai.ErrInvalidConfig)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/videos/"+url.PathEscape(taskID)+"/content", nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := c.requireSuccess(resp); err != nil {
		return nil, "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read OpenAI video content: %w", err)
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return body, contentType, nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	baseURL, err := normalizeBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: missing OpenAI API key", infraai.ErrInvalidConfig)
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode OpenAI request: %w", err)
		}
		reader = bytes.NewReader(data)
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
	message := sanitizeBody(body, c.apiKey)
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
		if data == "[DONE]" {
			if result.UsageStatus == "" {
				result.UsageStatus = infraai.UsageStatusUnavailable
			}
			return result, nil
		}
		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("decode OpenAI chat completion stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			result.PromptTokens = chunk.Usage.PromptTokens
			result.CompletionTokens = chunk.Usage.CompletionTokens
			result.TotalTokens = chunk.Usage.TotalTokens
			result.UsageStatus = infraai.UsageStatusReported
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
			if sink != nil {
				if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read OpenAI chat completion stream: %w", err)
	}
	if result.UsageStatus == "" {
		result.UsageStatus = infraai.UsageStatusUnavailable
	}
	return result, nil
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

func videoTaskFromPayload(payload map[string]any) *infraai.VideoTask {
	if payload == nil {
		return &infraai.VideoTask{}
	}
	task := &infraai.VideoTask{ID: stringFromAny(payload["id"]), Status: stringFromAny(payload["status"]), RawResponse: payload}
	if task.Status == "" {
		task.Status = stringFromAny(payload["state"])
	}
	if errorValue, ok := payload["error"].(map[string]any); ok {
		task.ErrorMessage = stringFromAny(errorValue["message"])
	}
	if task.ErrorMessage == "" {
		task.ErrorMessage = stringFromAny(payload["error_message"])
	}
	return task
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

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string               `json:"content"`
			ToolCalls []chatStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
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

func (r chatCompletionResponse) firstContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return contentText(r.Choices[0].Message.Content)
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

func contentText(value any) string {
	switch content := value.(type) {
	case string:
		return content
	case []any:
		var builder strings.Builder
		for _, item := range content {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := row["type"].(string); typ == "text" {
				if text, _ := row["text"].(string); text != "" {
					if builder.Len() > 0 {
						builder.WriteString("\n")
					}
					builder.WriteString(text)
				}
			}
		}
		return builder.String()
	default:
		return ""
	}
}

func normalizeBaseURL(value string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(value), "/")
	if raw == "" {
		raw = defaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid OpenAI base url", infraai.ErrInvalidConfig)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: invalid OpenAI base url scheme", infraai.ErrInvalidConfig)
	}
	return raw, nil
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
