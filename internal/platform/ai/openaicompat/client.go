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
	"time"

	platformai "admin_back_go/internal/platform/ai"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultTimeout = 30 * time.Second
)

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	timeout    time.Duration
}

func New(config Config) *Client {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:     strings.TrimSpace(config.APIKey),
		httpClient: httpClient,
		timeout:    timeout,
	}
}

func (c *Client) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", platformai.ErrInvalidConfig)
	}
	client := c
	if strings.TrimSpace(input.BaseURL) != "" || strings.TrimSpace(input.APIKey) != "" || input.TimeoutMs > 0 {
		timeout := c.timeout
		if input.TimeoutMs > 0 {
			timeout = time.Duration(input.TimeoutMs) * time.Millisecond
		}
		client = New(Config{
			BaseURL:    nonEmpty(input.BaseURL, c.baseURL),
			APIKey:     nonEmpty(input.APIKey, c.apiKey),
			HTTPClient: c.httpClient,
			Timeout:    timeout,
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
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := client.requireSuccess(resp); err != nil {
		return &platformai.TestConnectionResult{OK: false, Status: resp.Status, LatencyMs: latency, Message: err.Error()}, err
	}
	return &platformai.TestConnectionResult{OK: true, Status: resp.Status, LatencyMs: latency, Message: "ok"}, nil
}

func (c *Client) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", platformai.ErrInvalidConfig)
	}
	if strings.TrimSpace(input.Content) == "" {
		if len(inputAttachments(input.Inputs)) == 0 {
			return nil, fmt.Errorf("%w: missing message content", platformai.ErrInvalidConfig)
		}
	}
	model := inputString(input.Inputs, "model_id")
	if model == "" {
		return nil, fmt.Errorf("%w: missing model_id", platformai.ErrInvalidConfig)
	}
	body := chatCompletionRequest{
		Model:    model,
		Stream:   true,
		Messages: chatMessages(input),
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
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := c.requireSuccess(resp); err != nil {
		return nil, err
	}
	result, err := c.readChatCompletionStream(ctx, resp.Body, sink)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return fmt.Errorf("%w: OpenAI-compatible chat completion does not support stop in this MVP", platformai.ErrInvalidConfig)
}

func (c *Client) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	return nil, fmt.Errorf("%w: OpenAI-compatible knowledge sync is outside chat MVP", platformai.ErrInvalidConfig)
}

func (c *Client) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	return nil, fmt.Errorf("%w: OpenAI-compatible knowledge status is outside chat MVP", platformai.ErrInvalidConfig)
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	baseURL, err := normalizeBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: missing OpenAI API key", platformai.ErrInvalidConfig)
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

func (c *Client) requireSuccess(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("%w: %s", platformai.ErrUpstreamFailed, resp.Status)
	}
	message := sanitizeBody(body, c.apiKey)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %s %s", platformai.ErrUnauthorized, resp.Status, message)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s %s", platformai.ErrRateLimited, resp.Status, message)
	}
	return fmt.Errorf("%w: %s %s", platformai.ErrUpstreamFailed, resp.Status, message)
}

func (c *Client) readChatCompletionStream(ctx context.Context, body io.Reader, sink platformai.EventSink) (*platformai.ChatResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var answer strings.Builder
	result := &platformai.ChatResult{}
	for scanner.Scan() {
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
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			answer.WriteString(delta)
			result.Answer = strings.TrimSpace(answer.String())
			if sink != nil {
				if err := sink.Emit(ctx, platformai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read OpenAI chat completion stream: %w", err)
	}
	return result, nil
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
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
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (r chatCompletionResponse) firstContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return contentText(r.Choices[0].Message.Content)
}

func chatMessages(input platformai.ChatInput) []chatMessage {
	messages := []chatMessage{}
	if systemPrompt := inputString(input.Inputs, "system_prompt"); systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, historyMessages(input.Inputs)...)
	messages = append(messages, chatMessage{Role: "user", Content: userContent(input)})
	return messages
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

func userContent(input platformai.ChatInput) any {
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
		return "", fmt.Errorf("%w: invalid OpenAI base url", platformai.ErrInvalidConfig)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: invalid OpenAI base url scheme", platformai.ErrInvalidConfig)
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
