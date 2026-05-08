package dify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	platformai "admin_back_go/internal/platform/ai"
)

const defaultTimeout = 30 * time.Second

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

func New(config Config) (*Client, error) {
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: missing dify api key", platformai.ErrInvalidConfig)
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, httpClient: httpClient, timeout: timeout}, nil
}

func (c *Client) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	client := c
	if strings.TrimSpace(input.BaseURL) != "" || strings.TrimSpace(input.APIKey) != "" {
		timeout := c.timeout
		if input.TimeoutMs > 0 {
			timeout = time.Duration(input.TimeoutMs) * time.Millisecond
		}
		fresh, err := New(Config{BaseURL: nonEmpty(input.BaseURL, c.baseURL), APIKey: nonEmpty(input.APIKey, c.apiKey), HTTPClient: c.httpClient, Timeout: timeout})
		if err != nil {
			return nil, err
		}
		client = fresh
	}
	start := time.Now()
	req, err := client.newRequest(ctx, http.MethodGet, "/parameters", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.httpClient.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &platformai.TestConnectionResult{OK: false, Status: resp.Status, LatencyMs: latency, Message: "unauthorized"}, platformai.ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &platformai.TestConnectionResult{OK: false, Status: resp.Status, LatencyMs: latency, Message: readSmall(resp.Body)}, fmt.Errorf("%w: %s", platformai.ErrUpstreamFailed, resp.Status)
	}
	return &platformai.TestConnectionResult{OK: true, Status: resp.Status, LatencyMs: latency, Message: "ok"}, nil
}

func (c *Client) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	if strings.TrimSpace(input.Content) == "" {
		return nil, fmt.Errorf("%w: missing query", platformai.ErrInvalidConfig)
	}
	body := map[string]any{
		"query":              input.Content,
		"inputs":             input.Inputs,
		"user":               input.UserKey,
		"response_mode":      "streaming",
		"auto_generate_name": false,
	}
	if body["inputs"] == nil {
		body["inputs"] = map[string]any{}
	}
	if strings.TrimSpace(input.ConversationEngineID) != "" {
		body["conversation_id"] = strings.TrimSpace(input.ConversationEngineID)
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/chat-messages", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := requireSuccess(resp); err != nil {
		return nil, err
	}
	events, err := parseStreamEvents(resp.Body)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if out, ok := eventForSink(event); ok && sink != nil {
			if err := sink.Emit(ctx, out); err != nil {
				return nil, err
			}
		}
	}
	return streamResult(events)
}

func (c *Client) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	if strings.TrimSpace(input.EngineTaskID) == "" || strings.TrimSpace(input.UserKey) == "" {
		return fmt.Errorf("%w: missing stop task or user", platformai.ErrInvalidConfig)
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/chat-messages/"+url.PathEscape(input.EngineTaskID)+"/stop", map[string]any{"user": input.UserKey})
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	return requireSuccess(resp)
}

func (c *Client) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	if strings.TrimSpace(input.DatasetID) == "" {
		return nil, fmt.Errorf("%w: missing dataset id", platformai.ErrInvalidConfig)
	}
	if strings.TrimSpace(input.Document.Text) == "" {
		return nil, fmt.Errorf("%w: missing document text", platformai.ErrInvalidConfig)
	}
	body := map[string]any{
		"name":               nonEmpty(input.Document.Name, "Untitled"),
		"text":               input.Document.Text,
		"indexing_technique": "high_quality",
		"process_rule": map[string]any{
			"mode": "automatic",
		},
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/datasets/"+url.PathEscape(input.DatasetID)+"/document/create-by-text", body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := requireSuccess(resp); err != nil {
		return nil, err
	}
	var parsed struct {
		Document struct {
			ID             string `json:"id"`
			IndexingStatus string `json:"indexing_status"`
		} `json:"document"`
		Batch string `json:"batch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode dify knowledge response: %w", err)
	}
	return &platformai.KnowledgeSyncResult{
		EngineDatasetID:  input.DatasetID,
		EngineDocumentID: parsed.Document.ID,
		EngineBatch:      parsed.Batch,
		IndexingStatus:   parsed.Document.IndexingStatus,
	}, nil
}

func (c *Client) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	if strings.TrimSpace(input.DatasetID) == "" || strings.TrimSpace(input.DocumentID) == "" {
		return nil, fmt.Errorf("%w: missing dataset or document id", platformai.ErrInvalidConfig)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/datasets/"+url.PathEscape(input.DatasetID)+"/documents/"+url.PathEscape(input.DocumentID)+"/indexing-status", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if err := requireSuccess(resp); err != nil {
		return nil, err
	}
	var parsed struct {
		Data []struct {
			IndexingStatus string `json:"indexing_status"`
			Error          string `json:"error"`
		} `json:"data"`
		IndexingStatus string `json:"indexing_status"`
		Error          string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode dify knowledge status response: %w", err)
	}
	if len(parsed.Data) > 0 {
		return &platformai.KnowledgeStatusResult{IndexingStatus: parsed.Data[0].IndexingStatus, ErrorMessage: parsed.Data[0].Error}, nil
	}
	return &platformai.KnowledgeStatusResult{IndexingStatus: parsed.IndexingStatus, ErrorMessage: parsed.Error}, nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: nil dify client", platformai.ErrInvalidConfig)
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode dify request: %w", err)
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(endpoint), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) url(endpoint string) string {
	base, _ := url.Parse(c.baseURL)
	base.Path = path.Join(base.Path, endpoint)
	return base.String()
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", fmt.Errorf("%w: missing dify base url", platformai.ErrInvalidConfig)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid dify base url", platformai.ErrInvalidConfig)
	}
	return value, nil
}

func requireSuccess(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body := readSmall(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", platformai.ErrUnauthorized, body)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", platformai.ErrRateLimited, body)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s", platformai.ErrUpstreamTimeout, body)
	default:
		return fmt.Errorf("%w: %s %s", platformai.ErrUpstreamFailed, resp.Status, body)
	}
}

func readSmall(r io.Reader) string {
	if r == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(r, 4096))
	return strings.TrimSpace(string(data))
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
