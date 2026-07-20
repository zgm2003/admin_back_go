package aivideo

import (
	"context"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	StorageProviderCOS = "cos"
)

type HTTPService interface {
	Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error)
	Status(ctx context.Context, platform string, userID int64, id int64) (*StatusResponse, *apperror.Error)
	Content(ctx context.Context, platform string, userID int64, id int64) ([]byte, string, *apperror.Error)
	UploadReferenceMedia(ctx context.Context, input ReferenceMediaUploadInput) (*ReferenceMediaUploadResponse, *apperror.Error)
}

type CreateInput struct {
	Platform        string
	UserID          int64
	AgentID         int64
	ModelID         string
	Prompt          string
	DurationSeconds int
	Size            string
	ResolutionName  string
	GenerateAudio   *bool
	Watermark       *bool
}

type CreateResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type StatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type ReferenceMediaUploadInput struct {
	Platform  string
	UserID    int64
	MediaKind string
	FileName  string
	MimeType  string
	Body      []byte
}

type ReferenceMediaUploadResponse struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	StorageProvider string     `json:"storage_provider"`
	StorageKey      string     `json:"storage_key"`
	MimeType        string     `json:"mime_type"`
	MediaKind       string     `json:"media_kind"`
	Bytes           int64      `json:"bytes"`
	ExpiresAt       *time.Time `json:"expires_at"`
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
	CreateTask(ctx context.Context, task VideoTask) (int64, error)
	UpdateTask(ctx context.Context, platform string, userID int64, id int64, fields map[string]any) error
	GetTask(ctx context.Context, platform string, userID int64, id int64) (*VideoTask, error)
	LoadUploadConfig(ctx context.Context) (*UploadConfig, error)
}

type EngineConfig struct {
	EngineType infraai.EngineType
	BaseURL    string
	APIKey     string
}

type EngineFactory interface {
	NewVideoEngine(ctx context.Context, input EngineConfig) (infraai.VideoEngine, error)
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
	ObjectWriter  storagecos.ObjectWriter
	Now           func() time.Time
	Random        func([]byte) (int, error)
}
