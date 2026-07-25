package imagecompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultTimeout = 5 * time.Minute
	maxBodyBytes   = 128 << 20
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

func (c *Client) Capabilities() infraai.CapabilityMetadata {
	return infraai.CapabilityMetadata{
		SupportedUsageKeys:          []string{"usage.input_tokens", "usage.output_tokens", "usage.total_tokens"},
		SafeInputUpperBoundStrategy: "serialized_utf8_bytes_plus_framing",
		SupportsIdempotencyHeader:   true,
	}
}

func (c *Client) GenerateImages(ctx context.Context, input infraai.ImageInput) (*infraai.ImageResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI image client is nil", infraai.ErrInvalidConfig)
	}
	input = normalizeInput(input)
	if input.Model == "" {
		return nil, fmt.Errorf("%w: missing image model", infraai.ErrInvalidConfig)
	}
	if input.Prompt == "" {
		return nil, fmt.Errorf("%w: missing image prompt", infraai.ErrInvalidConfig)
	}
	var (
		req *http.Request
		err error
	)
	if len(input.InputAssets) > 0 {
		req, err = c.newEditRequest(ctx, input)
	} else {
		req, err = c.newGenerationRequest(ctx, input)
	}
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", infraai.ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	if !isSuccessStatus(resp.StatusCode) {
		body, err := readLimitedResponseBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read OpenAI image response: %w", err)
		}
		if err := c.requireSuccess(resp, body); err != nil {
			return nil, err
		}
	}
	return decodeImageResponse(resp.Body, imageMime(input.OutputFormat))
}

func (c *Client) newGenerationRequest(ctx context.Context, input infraai.ImageInput) (*http.Request, error) {
	body := imageRequest{
		Model:             input.Model,
		Prompt:            input.Prompt,
		Size:              input.Size,
		Quality:           input.Quality,
		OutputFormat:      input.OutputFormat,
		OutputCompression: input.OutputCompression,
		Moderation:        input.Moderation,
		N:                 input.N,
	}
	return c.newJSONRequest(ctx, http.MethodPost, "/images/generations", body)
}

func (c *Client) newEditRequest(ctx context.Context, input infraai.ImageInput) (*http.Request, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":         input.Model,
		"prompt":        input.Prompt,
		"size":          input.Size,
		"quality":       input.Quality,
		"output_format": input.OutputFormat,
		"moderation":    input.Moderation,
	}
	if input.N > 0 {
		fields["n"] = fmt.Sprintf("%d", input.N)
	}
	if input.OutputCompression != nil {
		fields["output_compression"] = fmt.Sprintf("%d", *input.OutputCompression)
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("build OpenAI image edit form: %w", err)
		}
	}
	for _, asset := range input.InputAssets {
		if err := writeFormFile(writer, "image", asset); err != nil {
			return nil, err
		}
	}
	if input.MaskAsset != nil {
		if err := writeFormFile(writer, "mask", *input.MaskAsset); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("build OpenAI image edit form: %w", err)
	}
	req, err := c.newRawRequest(ctx, http.MethodPost, "/images/edits", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func writeFormFile(writer *multipart.Writer, field string, asset infraai.ImageAsset) error {
	if len(asset.Data) == 0 {
		return fmt.Errorf("%w: image asset data is empty", infraai.ErrInvalidConfig)
	}
	name := strings.TrimSpace(asset.Name)
	if name == "" {
		name = "image" + extensionForMime(asset.MimeType)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartHeaderValue(field), escapeMultipartHeaderValue(filepath.Base(name))))
	if mimeType := strings.TrimSpace(asset.MimeType); strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		header.Set("Content-Type", mimeType)
	} else {
		header.Set("Content-Type", "application/octet-stream")
	}
	file, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("build OpenAI image edit form file: %w", err)
	}
	if _, err := file.Write(asset.Data); err != nil {
		return fmt.Errorf("build OpenAI image edit form file: %w", err)
	}
	return nil
}

func escapeMultipartHeaderValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func (c *Client) newJSONRequest(ctx context.Context, method string, endpoint string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode OpenAI image request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := c.newRawRequest(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) newRawRequest(ctx context.Context, method string, endpoint string, body io.Reader) (*http.Request, error) {
	baseURL, err := normalizeBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: missing OpenAI API key", infraai.ErrInvalidConfig)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI image request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if c.httpClient == nil {
		timeout := c.timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		c.httpClient = &http.Client{Timeout: timeout}
	}
	return req, nil
}

func (c *Client) requireSuccess(resp *http.Response, body []byte) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
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

func isSuccessStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func readLimitedResponseBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("%w: OpenAI image response too large", infraai.ErrUpstreamFailed)
	}
	return data, nil
}

type imageRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	N                 int    `json:"n,omitempty"`
}

type imageResponse struct {
	Data []struct {
		B64JSON       string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
		Size          string `json:"size"`
		Quality       string `json:"quality"`
		OutputFormat  string `json:"output_format"`
		Moderation    string `json:"moderation"`
	} `json:"data"`
	Size              string `json:"size"`
	Quality           string `json:"quality"`
	OutputFormat      string `json:"output_format"`
	OutputCompression int    `json:"output_compression"`
	Moderation        string `json:"moderation"`
	N                 int    `json:"n"`
	Usage             *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func decodeImageResponse(body io.Reader, fallbackMime string) (*infraai.ImageResult, error) {
	var (
		raw     bytes.Buffer
		payload imageResponse
	)
	decoder := json.NewDecoder(io.TeeReader(io.LimitReader(body, maxBodyBytes+1), &raw))
	if err := decoder.Decode(&payload); err != nil {
		if raw.Len() > maxBodyBytes {
			return nil, fmt.Errorf("%w: OpenAI image response too large", infraai.ErrUpstreamFailed)
		}
		return nil, fmt.Errorf("decode OpenAI image response: %w", err)
	}
	if raw.Len() > maxBodyBytes {
		return nil, fmt.Errorf("%w: OpenAI image response too large", infraai.ErrUpstreamFailed)
	}
	return imageResultFromPayload(payload, append([]byte(nil), raw.Bytes()...), fallbackMime)
}

func imageResultFromPayload(payload imageResponse, raw []byte, fallbackMime string) (*infraai.ImageResult, error) {
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("%w: OpenAI image response contains no data", infraai.ErrUpstreamFailed)
	}
	images := make([]infraai.GeneratedImage, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.B64JSON) == "" && strings.TrimSpace(item.URL) == "" {
			continue
		}
		mimeType := fallbackMime
		if item.OutputFormat != "" {
			mimeType = imageMime(item.OutputFormat)
		}
		images = append(images, infraai.GeneratedImage{
			B64JSON:       strings.TrimSpace(item.B64JSON),
			URL:           strings.TrimSpace(item.URL),
			MimeType:      mimeType,
			RevisedPrompt: strings.TrimSpace(item.RevisedPrompt),
		})
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: OpenAI image response contains no usable image", infraai.ErrUpstreamFailed)
	}
	result := &infraai.ImageResult{
		Images:       images,
		ActualParams: actualParams(payload),
		RawResponse:  raw,
		UsageStatus:  infraai.UsageStatusUnavailable,
	}
	if payload.Usage != nil {
		result.PromptTokens = payload.Usage.InputTokens
		result.CompletionTokens = payload.Usage.OutputTokens
		result.TotalTokens = payload.Usage.TotalTokens
		result.UsageStatus = infraai.UsageStatusReported
		if payload.Usage.InputTokens < 0 || payload.Usage.OutputTokens < 0 || payload.Usage.TotalTokens < 0 || ((payload.Usage.InputTokens != 0 || payload.Usage.OutputTokens != 0) && payload.Usage.TotalTokens != payload.Usage.InputTokens+payload.Usage.OutputTokens) {
			result.UsageStatus = infraai.UsageStatusUnavailable
		} else {
			items := make([]infraai.UsageItem, 0, 2)
			items = append(items, infraai.UsageItem{Category: infraai.UsageCategoryInput, Unit: "token", Quantity: int64(payload.Usage.InputTokens)})
			items = append(items, infraai.UsageItem{Category: infraai.UsageCategoryOutput, Unit: "token", Quantity: int64(payload.Usage.OutputTokens)})
			if payload.Usage.InputTokens == 0 && payload.Usage.OutputTokens == 0 && payload.Usage.TotalTokens > 0 {
				items = []infraai.UsageItem{{Category: infraai.UsageCategoryMedia, Unit: "token", Quantity: int64(payload.Usage.TotalTokens)}}
			}
			result.Usage, _ = infraai.NewUsageSnapshot(infraai.UsageStatusReported, raw, items)
			result.ResponseSHA256 = result.Usage.ResponseSHA256
		}
	}
	return result, nil
}

func actualParams(payload imageResponse) map[string]any {
	out := map[string]any{}
	if payload.Size != "" {
		out["size"] = payload.Size
	}
	if payload.Quality != "" {
		out["quality"] = payload.Quality
	}
	if payload.OutputFormat != "" {
		out["output_format"] = payload.OutputFormat
	}
	if payload.OutputCompression > 0 {
		out["output_compression"] = payload.OutputCompression
	}
	if payload.Moderation != "" {
		out["moderation"] = payload.Moderation
	}
	if payload.N > 0 {
		out["n"] = payload.N
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeInput(input infraai.ImageInput) infraai.ImageInput {
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Size = strings.TrimSpace(input.Size)
	input.Quality = strings.TrimSpace(input.Quality)
	input.OutputFormat = strings.TrimSpace(input.OutputFormat)
	input.Moderation = strings.TrimSpace(input.Moderation)
	if input.OutputFormat == "" {
		input.OutputFormat = "png"
	}
	if input.N <= 0 {
		input.N = 1
	}
	return input
}

func normalizeBaseURL(value string) (string, error) {
	return infraai.NormalizeOpenAIBaseURL(value, defaultBaseURL)
}

func sanitizeBody(body []byte, apiKey string) string {
	compact := strings.TrimSpace(string(bytes.TrimSpace(body)))
	if apiKey != "" {
		compact = strings.ReplaceAll(compact, apiKey, "[REDACTED]")
	}
	if len(compact) > 1024 {
		compact = compact[:1024]
	}
	return compact
}

func imageMime(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func extensionForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
