package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/telemetry"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type OpenAIDriver struct {
	client   *http.Client
	recorder telemetry.Recorder
}

type Option func(*OpenAIDriver)

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(driver *OpenAIDriver) {
		if recorder != nil {
			driver.recorder = recorder
		}
	}
}

func NewOpenAIDriver(client *http.Client, options ...Option) *OpenAIDriver {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	driver := &OpenAIDriver{client: client, recorder: telemetry.Noop()}
	for _, option := range options {
		if option != nil {
			option(driver)
		}
	}
	return driver
}

func (d *OpenAIDriver) Name() string { return DriverOpenAI }

func (d *OpenAIDriver) DefaultBaseURL() string { return defaultOpenAIBaseURL }

func (d *OpenAIDriver) ListModels(ctx context.Context, cfg Config) ([]Model, error) {
	startedAt := time.Now()
	models, err := d.listModels(ctx, cfg)
	status := "ok"
	if err != nil {
		status = "error"
	}
	attributes := telemetry.Attributes{
		"provider.name":     DriverOpenAI,
		"provider.modality": "discovery",
		"provider.status":   status,
	}
	d.recorder.Count("provider.requests", 1, attributes)
	d.recorder.Observe("provider.total_seconds", time.Since(startedAt).Seconds(), attributes)
	return models, err
}

func (d *OpenAIDriver) listModels(ctx context.Context, cfg Config) ([]Model, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("missing OpenAI API key")
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL, d.DefaultBaseURL())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request OpenAI models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read OpenAI models response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("OpenAI models failed: %s %s", resp.Status, sanitizeBody(body))
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode OpenAI models response: %w", err)
	}
	models := make([]Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		id, _ := item["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		object, _ := item["object"].(string)
		ownedBy, _ := item["owned_by"].(string)
		created := int64(0)
		if value, ok := item["created"].(float64); ok {
			created = int64(value)
		}
		models = append(models, Model{ID: id, Object: object, Created: created, OwnedBy: ownedBy})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func (d *OpenAIDriver) TestConnection(ctx context.Context, cfg Config) (*TestResult, error) {
	started := time.Now()
	models, err := d.ListModels(ctx, cfg)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return &TestResult{OK: false, Status: HealthFailed, LatencyMs: latency, Message: err.Error()}, err
	}
	return &TestResult{OK: true, Status: HealthOK, LatencyMs: latency, Message: fmt.Sprintf("OpenAI models reachable: %d", len(models)), ModelCount: len(models)}, nil
}

func normalizeBaseURL(value string, fallback string) (string, error) {
	return infraai.NormalizeOpenAIBaseURL(value, fallback)
}

func sanitizeBody(body []byte) string {
	compact := bytes.TrimSpace(body)
	if len(compact) > 512 {
		compact = compact[:512]
	}
	return string(compact)
}
