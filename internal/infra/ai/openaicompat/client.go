package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/apperror"
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
	APIProtocol       string
	FileOpener        infraai.PreparedFileOpener
	Logger            *slog.Logger
}

type Client struct {
	baseURL           string
	apiKey            string
	httpClient        *http.Client
	streamHTTPClient  *http.Client
	timeout           time.Duration
	streamIdleTimeout time.Duration
	apiProtocol       string
	fileOpener        infraai.PreparedFileOpener
	logger            *slog.Logger
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
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:           strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:            strings.TrimSpace(config.APIKey),
		httpClient:        httpClient,
		streamHTTPClient:  streamHTTPClient,
		timeout:           timeout,
		streamIdleTimeout: streamIdleTimeout,
		apiProtocol:       normalizeAPIProtocol(config.APIProtocol),
		fileOpener:        config.FileOpener,
		logger:            logger,
	}
}

func (c *Client) Capabilities() infraai.CapabilityMetadata {
	capabilities, _ := infraai.DefaultTransportCapabilities(infraai.EngineTypeOpenAI)
	return capabilities
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
			APIProtocol:       c.apiProtocol,
			FileOpener:        c.fileOpener,
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
	if err := client.requireSuccess(ctx, resp); err != nil {
		return &infraai.TestConnectionResult{OK: false, Status: resp.Status, LatencyMs: latency, Message: err.Error()}, err
	}
	return &infraai.TestConnectionResult{OK: true, Status: resp.Status, LatencyMs: latency, Message: "ok"}, nil
}

func (c *Client) StreamChat(ctx context.Context, input infraai.ChatInput, sink infraai.EventSink) (*infraai.ChatResult, error) {
	prepared, err := c.PrepareChat(ctx, input)
	if err != nil {
		return nil, err
	}
	preflightMetrics, err := c.PreflightPreparedChat(ctx, prepared)
	if err != nil {
		return nil, err
	}
	result, err := c.streamPreparedChat(ctx, infraai.PreparedChatRequest{Body: prepared, IdempotencyKey: input.IdempotencyKey}, sink, false)
	mergeFileInputMetrics(result, preflightMetrics)
	return result, err
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
		if !hasCurrentAttachments(input.Inputs) {
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
	messages, files, err := prepareChatMessages(input)
	if err != nil {
		return nil, err
	}
	var request any
	switch c.apiProtocol {
	case infraai.APIProtocolChatCompletions:
		if len(files) > 0 {
			return nil, fmt.Errorf("%w: native file input requires the Responses API protocol", infraai.ErrInvalidConfig)
		}
		body := chatCompletionRequest{
			Model:         model,
			Stream:        true,
			StreamOptions: &chatStreamOptions{IncludeUsage: true},
			Messages:      messages,
			Tools:         chatTools(input.Tools),
		}
		if temperature, ok := inputNumber(input.Inputs, "temperature"); ok {
			body.Temperature = &temperature
		}
		if input.EffectiveMaxOutputTokens < 0 {
			return nil, fmt.Errorf("%w: effective max output tokens must not be negative", infraai.ErrInvalidConfig)
		}
		if input.EffectiveMaxOutputTokens > 0 {
			body.MaxTokens = &input.EffectiveMaxOutputTokens
		}
		request = body
	case infraai.APIProtocolResponses:
		body, prepareErr := prepareResponsesRequest(input, model, messages)
		if prepareErr != nil {
			return nil, prepareErr
		}
		request = body
	default:
		return nil, fmt.Errorf("%w: unsupported OpenAI API protocol %q", infraai.ErrInvalidConfig, c.apiProtocol)
	}
	prepared, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAI request: %w", err)
	}
	if len(files) == 0 {
		if c.apiProtocol == infraai.APIProtocolResponses {
			return infraai.MarshalPreparedChatInlineEnvelope(infraai.PreparedChatInlineEnvelope{
				Schema: infraai.PreparedChatSchemaResponsesInlineV1, APIProtocol: c.apiProtocol, Request: prepared,
			})
		}
		return prepared, nil
	}
	if c.fileOpener == nil {
		return nil, fmt.Errorf("%w: prepared file opener is missing", infraai.ErrInvalidConfig)
	}
	return infraai.MarshalPreparedChatFileManifest(infraai.PreparedChatFileManifest{
		Schema: infraai.PreparedChatSchemaResponsesFileManifestV1, APIProtocol: c.apiProtocol,
		Request: prepared, Files: files,
	})
}

func (c *Client) PreflightPreparedChat(ctx context.Context, body []byte) (*infraai.FileInputMetrics, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	schema, err := infraai.DetectPreparedChatSchema(body)
	if err != nil {
		return nil, err
	}
	if schema == infraai.PreparedChatSchemaFileManifestV1 || schema == infraai.PreparedChatSchemaResponsesFileManifestV1 {
		if c == nil || c.fileOpener == nil {
			return nil, fmt.Errorf("%w: prepared file opener is missing", infraai.ErrInvalidConfig)
		}
		manifest, parseErr := infraai.ParsePreparedChatFileManifest(body)
		if parseErr != nil {
			return nil, parseErr
		}
		metrics := &infraai.FileInputMetrics{}
		for _, file := range manifest.Files {
			headStartedAt := time.Now()
			metadata, headErr := c.fileOpener.Head(ctx, infraai.PreparedFileOpenInput{
				ObjectKey: file.ObjectKey, ETag: file.ETag, Size: file.Size,
			})
			metrics.COSHeadMS += time.Since(headStartedAt).Milliseconds()
			if headErr != nil {
				return metrics, headErr
			}
			if metadataErr := preparedFileMetadataError(file, metadata); metadataErr != nil {
				return metrics, metadataErr
			}
		}
		return metrics, nil
	}
	return nil, nil
}

func mergeFileInputMetrics(result *infraai.ChatResult, preflight *infraai.FileInputMetrics) {
	if result == nil || preflight == nil {
		return
	}
	if result.FileInputMetrics == nil {
		metrics := *preflight
		result.FileInputMetrics = &metrics
		return
	}
	result.FileInputMetrics.COSHeadMS += preflight.COSHeadMS
}

func (c *Client) StreamPreparedChat(ctx context.Context, input infraai.PreparedChatRequest, sink infraai.EventSink) (*infraai.ChatResult, error) {
	return c.streamPreparedChat(ctx, input, sink, true)
}

func (c *Client) streamPreparedChat(ctx context.Context, input infraai.PreparedChatRequest, sink infraai.EventSink, requireKey bool) (*infraai.ChatResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrInvalidConfig)
	}
	schema, schemaErr := infraai.DetectPreparedChatSchema(input.Body)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: invalid prepared chat request", infraai.ErrInvalidConfig)
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if requireKey && key == "" {
		return nil, fmt.Errorf("%w: missing prepared chat idempotency key", infraai.ErrInvalidConfig)
	}
	apiProtocol, protocolErr := preparedRequestAPIProtocol(input.Body)
	if protocolErr != nil {
		return nil, protocolErr
	}
	var requestBody io.ReadCloser
	var contentLength int64
	var materialized *MaterializedRequest
	switch schema {
	case infraai.PreparedChatSchemaInlineV1:
		requestBody = io.NopCloser(bytes.NewReader(input.Body))
		contentLength = int64(len(input.Body))
	case infraai.PreparedChatSchemaResponsesInlineV1:
		envelope, parseErr := infraai.ParsePreparedChatInlineEnvelope(input.Body)
		if parseErr != nil {
			return nil, parseErr
		}
		requestBody = io.NopCloser(bytes.NewReader(envelope.Request))
		contentLength = int64(len(envelope.Request))
	case infraai.PreparedChatSchemaFileManifestV1, infraai.PreparedChatSchemaResponsesFileManifestV1:
		if c.fileOpener == nil {
			return nil, fmt.Errorf("%w: prepared file opener is missing", infraai.ErrInvalidConfig)
		}
		manifest, parseErr := infraai.ParsePreparedChatFileManifest(input.Body)
		if parseErr != nil {
			return nil, parseErr
		}
		value, materializeErr := MaterializeFileManifest(ctx, manifest, c.fileOpener)
		if materializeErr != nil {
			return nil, materializeErr
		}
		materialized = &value
		requestBody = value.Body
		contentLength = value.ContentLength
	default:
		return nil, fmt.Errorf("%w: unsupported prepared chat schema", infraai.ErrInvalidConfig)
	}
	endpoint := "/chat/completions"
	if apiProtocol == infraai.APIProtocolResponses {
		endpoint = "/responses"
	}
	req, err := c.newJSONReaderRequest(ctx, http.MethodPost, endpoint, requestBody, contentLength)
	if err != nil {
		_ = requestBody.Close()
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
	var fileMetrics *infraai.FileInputMetrics
	if materialized != nil {
		materialization := <-materialized.Result
		if materialization.Err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "", materialization.Err)
		}
		metrics := materialization.Metrics
		fileMetrics = &metrics
	}
	if err != nil {
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, "", fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err))
	}
	defer resp.Body.Close()
	providerRequestID := strings.TrimSpace(resp.Header.Get("X-Request-Id"))
	if err := c.requireSuccess(ctx, resp); err != nil {
		if (schema == infraai.PreparedChatSchemaFileManifestV1 || schema == infraai.PreparedChatSchemaResponsesFileManifestV1) && isExplicitFilePartRejection(err) {
			err = apperror.Wrap(
				"ai.provider.file_part_rejected", apperror.CategoryDependency, http.StatusBadGateway, apperror.Permanent,
				"ai.provider.file_part_rejected", nil, "上游渠道拒绝文件内容，请检查渠道文件协议", err,
			)
		}
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeRejected, providerRequestID, err)
	}
	watcher := newStreamIdleWatcher(streamIdleTimeout, resp.Body.Close)
	defer watcher.Stop()
	var result *infraai.ChatResult
	if apiProtocol == infraai.APIProtocolResponses {
		result, err = c.readResponsesStream(ctx, resp.Body, sink, func() { watcher.Touch(streamIdleTimeout) })
	} else {
		result, err = c.readChatCompletionStream(ctx, resp.Body, sink, func() { watcher.Touch(streamIdleTimeout) })
	}
	if err != nil {
		if watcher.TimedOut() {
			return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, providerRequestID, fmt.Errorf("%w: OpenAI %s stream idle timeout after %s", context.DeadlineExceeded, apiProtocol, streamIdleTimeout))
		}
		if apiProtocol == infraai.APIProtocolResponses && isResponsesTerminalStreamError(err) && result != nil {
			result.ProviderRequestID = providerRequestID
			result.DispatchState = infraai.DispatchStateDispatched
			result.FileInputMetrics = fileMetrics
			return result, infraai.NewProviderError(infraai.ProviderOutcomeRejected, providerRequestID, err)
		}
		return nil, infraai.NewProviderError(infraai.ProviderOutcomeUnknown, providerRequestID, err)
	}
	result.ProviderRequestID = providerRequestID
	result.DispatchState = infraai.DispatchStateDispatched
	result.FileInputMetrics = fileMetrics
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

func (c *Client) newJSONReaderRequest(ctx context.Context, method string, endpoint string, body io.ReadCloser, contentLength int64) (*http.Request, error) {
	baseURL, err := normalizeBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: missing OpenAI API key", infraai.ErrInvalidConfig)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI request: %w", err)
	}
	req.ContentLength = contentLength
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
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

func (c *Client) requireSuccess(ctx context.Context, resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("%w: %s", infraai.ErrUpstreamFailed, resp.Status)
	}
	message := upstreamHTTPErrorMessage(body, c.apiKey)
	metadata := extractUpstreamErrorMetadata(body)
	metadata.code = sanitizeBody([]byte(metadata.code), c.apiKey)
	metadata.kind = sanitizeBody([]byte(metadata.kind), c.apiKey)
	metadata.param = sanitizeBody([]byte(metadata.param), c.apiKey)
	cause := error(infraai.ErrUpstreamFailed)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		cause = infraai.ErrUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		cause = infraai.ErrRateLimited
	}
	if c.logger != nil {
		c.logger.WarnContext(ctx, "AI provider request rejected",
			"status_code", resp.StatusCode,
			"provider_status", resp.Status,
			"error_code", metadata.code,
			"error_type", metadata.kind,
			"error_param", metadata.param,
			"message", message,
		)
	}
	return &upstreamResponseError{
		cause: cause, statusCode: resp.StatusCode, status: resp.Status, message: message,
		code: metadata.code, kind: metadata.kind, param: metadata.param,
	}
}

type upstreamResponseError struct {
	cause      error
	statusCode int
	status     string
	message    string
	code       string
	kind       string
	param      string
}

func (err *upstreamResponseError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v: %s %s", err.cause, err.status, err.message)
}

func (err *upstreamResponseError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func isExplicitFilePartRejection(err error) bool {
	var responseErr *upstreamResponseError
	if !errors.As(err, &responseErr) || (responseErr.statusCode != http.StatusBadRequest && responseErr.statusCode != http.StatusUnprocessableEntity) {
		return false
	}
	for _, field := range []string{responseErr.code, responseErr.kind, responseErr.param} {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), "file") {
			return true
		}
	}
	message := strings.ToLower(strings.TrimSpace(responseErr.message))
	if !strings.Contains(message, "file") {
		return false
	}
	for _, marker := range []string{"not supported", "unsupported", "invalid", "rejected", "not allowed", "only supports"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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
			result.Usage = streamUsageSnapshot(chunk.Usage)
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
				if deliveryEnabled && toolCallDeltaDeliverable(call) {
					err := sink.Emit(ctx, infraai.Event{Type: "tool_delta", Payload: map[string]any{
						"tool_call_id":    strings.TrimSpace(call.ID),
						"name":            strings.TrimSpace(call.Function.Name),
						"arguments_delta": call.Function.Arguments,
					}})
					if err != nil {
						if infraai.IsFatalEventSinkError(err) {
							return nil, err
						}
						deliveryEnabled = false
					}
				}
			}
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			answer.WriteString(delta)
			result.Answer = strings.TrimSpace(answer.String())
			if deliveryEnabled {
				if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: delta, Payload: map[string]any{"delta": delta}}); err != nil {
					if infraai.IsFatalEventSinkError(err) {
						return nil, err
					}
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

func toolCallDeltaDeliverable(call chatStreamToolCall) bool {
	return strings.TrimSpace(call.ID) != "" || strings.TrimSpace(call.Function.Name) != "" || call.Function.Arguments != ""
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
	Usage *chatCompletionUsage `json:"usage"`
}

type chatCompletionUsage struct {
	PromptTokens             *int                  `json:"prompt_tokens"`
	CompletionTokens         *int                  `json:"completion_tokens"`
	TotalTokens              *int                  `json:"total_tokens"`
	CacheCreationInputTokens *int                  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int                  `json:"cache_read_input_tokens"`
	CacheCreation            *cacheCreationDetails `json:"cache_creation,omitempty"`
	PromptDetails            *promptTokenDetails   `json:"prompt_tokens_details,omitempty"`
	hasDuplicateKey          bool
}

func (usage *chatCompletionUsage) UnmarshalJSON(data []byte) error {
	duplicate, err := hasDuplicateJSONKey(data)
	if err != nil {
		return err
	}
	type plainUsage chatCompletionUsage
	if err := json.Unmarshal(data, (*plainUsage)(usage)); err != nil {
		return err
	}
	usage.hasDuplicateKey = duplicate
	return nil
}

type promptTokenDetails struct {
	CachedTokens             *int                  `json:"cached_tokens"`
	CacheCreationInputTokens *int                  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int                  `json:"cache_read_input_tokens"`
	CacheCreation            *cacheCreationDetails `json:"cache_creation,omitempty"`
}

type cacheCreationDetails struct {
	Ephemeral5mInputTokens *int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens"`
}

func usageInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func streamUsageSnapshot(usage *chatCompletionUsage) infraai.UsageSnapshot {
	if usage == nil || usage.hasDuplicateKey {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	details, ok := mergedPromptTokenDetails(usage)
	if !ok {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	return tokenUsageSnapshot(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, details)
}

func mergedPromptTokenDetails(usage *chatCompletionUsage) (*promptTokenDetails, bool) {
	if usage == nil {
		return nil, true
	}
	var merged promptTokenDetails
	hasDetails := usage.PromptDetails != nil || usage.CacheCreationInputTokens != nil || usage.CacheReadInputTokens != nil || usage.CacheCreation != nil
	if usage.PromptDetails != nil {
		merged = *usage.PromptDetails
		if usage.PromptDetails.CacheCreation != nil {
			creation := *usage.PromptDetails.CacheCreation
			merged.CacheCreation = &creation
		}
	}
	if usage.CacheReadInputTokens != nil {
		if !consistentOptionalInt(usage.CacheReadInputTokens, merged.CachedTokens) || !consistentOptionalInt(usage.CacheReadInputTokens, merged.CacheReadInputTokens) {
			return nil, false
		}
		if merged.CachedTokens == nil && merged.CacheReadInputTokens == nil {
			merged.CacheReadInputTokens = usage.CacheReadInputTokens
		}
	}
	if !mergeOptionalInt(&merged.CacheCreationInputTokens, usage.CacheCreationInputTokens) {
		return nil, false
	}
	if usage.CacheCreation != nil {
		if merged.CacheCreation != nil && !sameCacheCreationDetails(merged.CacheCreation, usage.CacheCreation) {
			return nil, false
		}
		if merged.CacheCreation == nil {
			creation := *usage.CacheCreation
			merged.CacheCreation = &creation
		}
	}
	if !hasDetails {
		return nil, true
	}
	return &merged, true
}

func consistentOptionalInt(authoritative, variant *int) bool {
	return authoritative == nil || variant == nil || *authoritative == *variant
}

func mergeOptionalInt(target **int, value *int) bool {
	if value == nil {
		return true
	}
	if *target != nil {
		return **target == *value
	}
	*target = value
	return true
}

func sameCacheCreationDetails(left, right *cacheCreationDetails) bool {
	return sameOptionalInt(left.Ephemeral5mInputTokens, right.Ephemeral5mInputTokens) &&
		sameOptionalInt(left.Ephemeral1hInputTokens, right.Ephemeral1hInputTokens)
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func hasDuplicateJSONKey(data []byte) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	duplicate, err := scanJSONValue(decoder)
	if err != nil {
		return false, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false, fmt.Errorf("invalid trailing JSON data")
	}
	return duplicate, nil
}

func scanJSONValue(decoder *json.Decoder) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return false, nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		duplicate := false
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return false, fmt.Errorf("invalid JSON object key")
			}
			if _, exists := seen[key]; exists {
				duplicate = true
			}
			seen[key] = struct{}{}
			childDuplicate, err := scanJSONValue(decoder)
			if err != nil {
				return false, err
			}
			duplicate = duplicate || childDuplicate
		}
		if _, err := decoder.Token(); err != nil {
			return false, err
		}
		return duplicate, nil
	case '[':
		duplicate := false
		for decoder.More() {
			childDuplicate, err := scanJSONValue(decoder)
			if err != nil {
				return false, err
			}
			duplicate = duplicate || childDuplicate
		}
		if _, err := decoder.Token(); err != nil {
			return false, err
		}
		return duplicate, nil
	default:
		return false, fmt.Errorf("unexpected JSON delimiter")
	}
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
	var writeItems []infraai.UsageItem
	if details != nil {
		cached, explicitRead := usageInt(details.CachedTokens), usageInt(details.CacheReadInputTokens)
		if details.CachedTokens != nil && details.CacheReadInputTokens != nil {
			return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
		}
		read = cached + explicitRead
		hasRead = details.CachedTokens != nil || details.CacheReadInputTokens != nil
		if details.CacheCreation != nil {
			fiveMinutes := usageInt(details.CacheCreation.Ephemeral5mInputTokens)
			oneHour := usageInt(details.CacheCreation.Ephemeral1hInputTokens)
			if fiveMinutes < 0 || oneHour < 0 {
				return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
			}
			write = fiveMinutes + oneHour
			if details.CacheCreationInputTokens != nil && *details.CacheCreationInputTokens != write {
				return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
			}
			if details.CacheCreation.Ephemeral5mInputTokens != nil {
				writeItems = append(writeItems, infraai.UsageItem{Category: infraai.UsageCategoryCacheWrite, Unit: "token", TierKey: "5m", Quantity: int64(fiveMinutes)})
			}
			if details.CacheCreation.Ephemeral1hInputTokens != nil {
				writeItems = append(writeItems, infraai.UsageItem{Category: infraai.UsageCategoryCacheWrite, Unit: "token", TierKey: "1h", Quantity: int64(oneHour)})
			}
			hasWrite = len(writeItems) > 0
		} else if details.CacheCreationInputTokens != nil {
			write = *details.CacheCreationInputTokens
			hasWrite = true
			writeItems = append(writeItems, infraai.UsageItem{Category: infraai.UsageCategoryCacheWrite, Unit: "token", Quantity: int64(write)})
		}
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
			items = append(items, writeItems...)
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

func prepareChatMessages(input infraai.ChatInput) ([]chatMessage, []infraai.PreparedFileRef, error) {
	messages := []chatMessage{}
	files := make([]infraai.PreparedFileRef, 0)
	if systemPrompt := inputString(input.Inputs, "system_prompt"); systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	history, err := historyMessages(input.Inputs, &files)
	if err != nil {
		return nil, nil, err
	}
	messages = append(messages, history...)
	content, err := chatMessageContent(strings.TrimSpace(input.Content), input.Inputs["attachments"], &files)
	if err != nil {
		return nil, nil, err
	}
	messages = append(messages, chatMessage{Role: "user", Content: content})
	if toolCalls := chatAssistantToolCalls(input.ToolCalls); len(toolCalls) > 0 {
		messages = append(messages, chatMessage{Role: "assistant", Content: nil, ToolCalls: toolCalls})
	}
	for _, output := range input.ToolOutputs {
		if strings.TrimSpace(output.CallID) == "" || strings.TrimSpace(output.Name) == "" {
			continue
		}
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: strings.TrimSpace(output.CallID), Content: strings.TrimSpace(output.Output)})
	}
	return messages, files, nil
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

func historyMessages(inputs map[string]any, files *[]infraai.PreparedFileRef) ([]chatMessage, error) {
	raw := inputs["history"]
	rows := make([]map[string]any, 0)
	switch values := raw.(type) {
	case []map[string]any:
		rows = values
	case []map[string]string:
		rows = make([]map[string]any, 0, len(values))
		for _, value := range values {
			rows = append(rows, map[string]any{"role": value["role"], "content": value["content"]})
		}
	default:
		return nil, nil
	}
	messages := make([]chatMessage, 0, len(rows))
	for _, row := range rows {
		role := strings.TrimSpace(stringFromAny(row["role"]))
		content := strings.TrimSpace(stringFromAny(row["content"]))
		switch role {
		case "user", "assistant", "system":
			preparedContent, err := chatMessageContent(content, row["attachments"], files)
			if err != nil {
				return nil, err
			}
			if content == "" {
				if parts, ok := preparedContent.([]any); !ok || len(parts) == 0 {
					continue
				}
			}
			messages = append(messages, chatMessage{Role: role, Content: preparedContent})
		}
	}
	return messages, nil
}

func chatMessageContent(text string, rawAttachments any, files *[]infraai.PreparedFileRef) (any, error) {
	attachments := attachmentRows(rawAttachments)
	if len(attachments) == 0 {
		return text, nil
	}
	parts := make([]any, 0, len(attachments)+1)
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, row := range attachments {
		typeValue := strings.TrimSpace(stringFromAny(row["type"]))
		switch typeValue {
		case "image":
			url := strings.TrimSpace(stringFromAny(row["url"]))
			if url == "" {
				continue
			}
			parts = append(parts, map[string]any{
				"type": "image_url", "image_url": map[string]any{"url": url},
			})
		case "file":
			file, err := preparedFileRefFromAttachment(row, len(*files)+1)
			if err != nil {
				return nil, err
			}
			*files = append(*files, file)
			parts = append(parts, struct {
				Type string `json:"type"`
				Ref  string `json:"ref"`
			}{Type: "file_ref", Ref: file.Ref})
		}
	}
	if len(parts) == 0 {
		return text, nil
	}
	return parts, nil
}

func hasCurrentAttachments(inputs map[string]any) bool {
	if inputs == nil {
		return false
	}
	for _, row := range attachmentRows(inputs["attachments"]) {
		switch strings.TrimSpace(stringFromAny(row["type"])) {
		case "image", "file":
			return true
		}
	}
	return false
}

func attachmentRows(raw any) []map[string]any {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	rows := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if row, ok := value.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func preparedFileRefFromAttachment(row map[string]any, sequence int) (infraai.PreparedFileRef, error) {
	size, ok := positiveInt64(row["size"])
	file := infraai.PreparedFileRef{
		Ref:       fmt.Sprintf("file-%d", sequence),
		ObjectKey: strings.TrimSpace(stringFromAny(row["object_key"])),
		ETag:      strings.TrimSpace(stringFromAny(row["etag"])),
		Size:      size,
		MIMEType:  strings.ToLower(strings.TrimSpace(stringFromAny(row["mime_type"]))),
		Filename:  strings.TrimSpace(stringFromAny(row["name"])),
	}
	if !ok || file.ObjectKey == "" || file.ETag == "" || file.MIMEType == "" || file.Filename == "" {
		return infraai.PreparedFileRef{}, fmt.Errorf("%w: native file attachment facts are incomplete", infraai.ErrInvalidConfig)
	}
	return file, nil
}

func positiveInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number > 0
	case int64:
		return number, number > 0
	case uint64:
		if number == 0 || number > math.MaxInt64 {
			return 0, false
		}
		return int64(number), true
	case float64:
		if number <= 0 || number > math.MaxInt64 || number != math.Trunc(number) {
			return 0, false
		}
		return int64(number), true
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
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

type upstreamErrorMetadata struct {
	code  string
	kind  string
	param string
}

func extractUpstreamErrorMetadata(body []byte) upstreamErrorMetadata {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return upstreamErrorMetadata{}
	}
	source := payload
	if nested, ok := payload["error"].(map[string]any); ok {
		source = nested
	}
	return upstreamErrorMetadata{
		code:  stringFromAny(source["code"]),
		kind:  stringFromAny(source["type"]),
		param: stringFromAny(source["param"]),
	}
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
