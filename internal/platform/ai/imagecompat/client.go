package imagecompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	platformai "admin_back_go/internal/platform/ai"
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

func (c *Client) GenerateImages(ctx context.Context, input platformai.ImageInput) (*platformai.ImageResult, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: OpenAI image client is nil", platformai.ErrInvalidConfig)
	}
	input = normalizeInput(input)
	if input.Model == "" {
		return nil, fmt.Errorf("%w: missing image model", platformai.ErrInvalidConfig)
	}
	if input.Prompt == "" {
		return nil, fmt.Errorf("%w: missing image prompt", platformai.ErrInvalidConfig)
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
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", platformai.ErrUpstreamFailed, err)
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

func (c *Client) newGenerationRequest(ctx context.Context, input platformai.ImageInput) (*http.Request, error) {
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

func (c *Client) newEditRequest(ctx context.Context, input platformai.ImageInput) (*http.Request, error) {
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

func writeFormFile(writer *multipart.Writer, field string, asset platformai.ImageAsset) error {
	if len(asset.Data) == 0 {
		return fmt.Errorf("%w: image asset data is empty", platformai.ErrInvalidConfig)
	}
	name := strings.TrimSpace(asset.Name)
	if name == "" {
		name = "image" + extensionForMime(asset.MimeType)
	}
	file, err := writer.CreateFormFile(field, filepath.Base(name))
	if err != nil {
		return fmt.Errorf("build OpenAI image edit form file: %w", err)
	}
	if _, err := file.Write(asset.Data); err != nil {
		return fmt.Errorf("build OpenAI image edit form file: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("%w: missing OpenAI API key", platformai.ErrInvalidConfig)
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
		return fmt.Errorf("%w: %s %s", platformai.ErrUnauthorized, resp.Status, message)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s %s", platformai.ErrRateLimited, resp.Status, message)
	}
	return fmt.Errorf("%w: %s %s", platformai.ErrUpstreamFailed, resp.Status, message)
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
		return nil, fmt.Errorf("%w: OpenAI image response too large", platformai.ErrUpstreamFailed)
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
}

func parseImageResponse(body []byte, fallbackMime string) (*platformai.ImageResult, error) {
	var payload imageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode OpenAI image response: %w", err)
	}
	return imageResultFromPayload(payload, append([]byte(nil), body...), fallbackMime)
}

func decodeImageResponse(body io.Reader, fallbackMime string) (*platformai.ImageResult, error) {
	var (
		raw     bytes.Buffer
		payload imageResponse
	)
	decoder := json.NewDecoder(io.TeeReader(io.LimitReader(body, maxBodyBytes+1), &raw))
	if err := decoder.Decode(&payload); err != nil {
		if raw.Len() > maxBodyBytes {
			return nil, fmt.Errorf("%w: OpenAI image response too large", platformai.ErrUpstreamFailed)
		}
		return nil, fmt.Errorf("decode OpenAI image response: %w", err)
	}
	if raw.Len() > maxBodyBytes {
		return nil, fmt.Errorf("%w: OpenAI image response too large", platformai.ErrUpstreamFailed)
	}
	return imageResultFromPayload(payload, append([]byte(nil), raw.Bytes()...), fallbackMime)
}

func imageResultFromPayload(payload imageResponse, raw []byte, fallbackMime string) (*platformai.ImageResult, error) {
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("%w: OpenAI image response contains no data", platformai.ErrUpstreamFailed)
	}
	images := make([]platformai.GeneratedImage, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.B64JSON) == "" && strings.TrimSpace(item.URL) == "" {
			continue
		}
		mimeType := fallbackMime
		if item.OutputFormat != "" {
			mimeType = imageMime(item.OutputFormat)
		}
		images = append(images, platformai.GeneratedImage{
			B64JSON:       strings.TrimSpace(item.B64JSON),
			URL:           strings.TrimSpace(item.URL),
			MimeType:      mimeType,
			RevisedPrompt: strings.TrimSpace(item.RevisedPrompt),
		})
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: OpenAI image response contains no usable image", platformai.ErrUpstreamFailed)
	}
	return &platformai.ImageResult{
		Images:       images,
		ActualParams: actualParams(payload),
		RawResponse:  raw,
	}, nil
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

func normalizeInput(input platformai.ImageInput) platformai.ImageInput {
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
