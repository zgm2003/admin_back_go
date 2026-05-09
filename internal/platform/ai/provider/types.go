package provider

import "context"

const (
	DriverOpenAI = "openai"

	HealthUnknown = "unknown"
	HealthOK      = "ok"
	HealthFailed  = "failed"
)

type Config struct {
	Driver    string
	BaseURL   string
	APIKey    string
	TimeoutMs int
}

type Model struct {
	ID      string
	Object  string
	Created int64
	OwnedBy string
	Raw     map[string]any
}

type TestResult struct {
	OK         bool
	Status     string
	LatencyMs  int64
	Message    string
	ModelCount int
}

type Driver interface {
	Name() string
	DefaultBaseURL() string
	ListModels(ctx context.Context, cfg Config) ([]Model, error)
	TestConnection(ctx context.Context, cfg Config) (*TestResult, error)
}
