package aiaudio

import (
	"context"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
)

type HTTPService interface {
	Generate(ctx context.Context, input GenerateInput) (*GenerateResponse, *apperror.Error)
}

type GenerateInput struct {
	Platform       string
	UserID         int64
	AgentID        int64
	ModelID        string
	Prompt         string
	Voice          string
	ResponseFormat string
	Speed          *float64
	Instructions   string
}

type GenerateResponse struct {
	Body        []byte
	ContentType string
}

type AgentRuntime struct {
	AgentID          int64
	ProviderID       int64
	ModelID          string
	ModelDisplayName string
	SystemPrompt     string
	ScenesJSON       string
	EngineType       string
	EngineBaseURL    string
	EngineAPIKeyEnc  string
	AgentStatus      int
	EngineStatus     int
}

type Repository interface {
	AgentForRuntime(ctx context.Context, agentID int64) (*AgentRuntime, error)
}

type EngineConfig struct {
	EngineType infraai.EngineType
	BaseURL    string
	APIKey     string
}

type EngineFactory interface {
	NewAudioEngine(ctx context.Context, input EngineConfig) (infraai.AudioEngine, error)
}

type Secretbox interface {
	Decrypt(cipherText string) (string, error)
}

type RunRecorder interface {
	airun.Recorder
}

type Dependencies struct {
	Repository    Repository
	Secretbox     Secretbox
	EngineFactory EngineFactory
	RunRecorder   RunRecorder
	Now           func() time.Time
}
