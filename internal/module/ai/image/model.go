package aiimage

import (
	"time"

	"admin_back_go/internal/module/ai/officialmodel"
)

type ImageTask struct {
	ID                       uint64     `gorm:"column:id"`
	Platform                 string     `gorm:"column:platform"`
	UserID                   uint64     `gorm:"column:user_id"`
	RequestID                string     `gorm:"column:request_id"`
	RequestFingerprint       []byte     `gorm:"column:request_fingerprint"`
	RequestIdentityStatus    string     `gorm:"column:request_identity_status"`
	RequestIdentityMarker    string     `gorm:"column:request_identity_marker"`
	RunID                    int64      `gorm:"column:run_id"`
	AgentID                  uint64     `gorm:"column:agent_id"`
	AgentNameSnapshot        string     `gorm:"column:agent_name_snapshot"`
	ProviderIDSnapshot       uint64     `gorm:"column:provider_id_snapshot"`
	ProviderNameSnapshot     string     `gorm:"column:provider_name_snapshot"`
	ModelIDSnapshot          string     `gorm:"column:model_id_snapshot"`
	ModelDisplayNameSnapshot string     `gorm:"column:model_display_name_snapshot"`
	Prompt                   string     `gorm:"column:prompt"`
	Size                     string     `gorm:"column:size"`
	Quality                  string     `gorm:"column:quality"`
	OutputFormat             string     `gorm:"column:output_format"`
	OutputCompression        *int       `gorm:"column:output_compression"`
	Moderation               string     `gorm:"column:moderation"`
	N                        int        `gorm:"column:n"`
	Status                   string     `gorm:"column:status"`
	LeaseOwner               *string    `gorm:"column:lease_owner"`
	LeaseToken               uint64     `gorm:"column:lease_token"`
	LeaseExpiresAt           *time.Time `gorm:"column:lease_expires_at"`
	ErrorMessage             string     `gorm:"column:error_message"`
	LastErrorCode            string     `gorm:"column:last_error_code"`
	ActualParamsJSON         *string    `gorm:"column:actual_params_json"`
	RawResponseJSON          *string    `gorm:"column:raw_response_json"`
	IsFavorite               int        `gorm:"column:is_favorite"`
	IsDel                    int        `gorm:"column:is_del;default:2"`
	FinishedAt               *time.Time `gorm:"column:finished_at"`
	ElapsedMS                int        `gorm:"column:elapsed_ms"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at"`
}

func (ImageTask) TableName() string { return "ai_image_tasks" }

type ImageFile struct {
	ID              uint64    `gorm:"column:id"`
	TaskID          uint64    `gorm:"column:task_id"`
	Role            string    `gorm:"column:role"`
	SortOrder       int       `gorm:"column:sort_order"`
	StorageProvider string    `gorm:"column:storage_provider"`
	StorageKey      string    `gorm:"column:storage_key"`
	StorageURL      string    `gorm:"column:storage_url"`
	MimeType        string    `gorm:"column:mime_type"`
	Width           int       `gorm:"column:width"`
	Height          int       `gorm:"column:height"`
	SizeBytes       int64     `gorm:"column:size_bytes"`
	RelatedFileID   *uint64   `gorm:"column:related_file_id"`
	RevisedPrompt   *string   `gorm:"column:revised_prompt"`
	IsDel           int       `gorm:"column:is_del;default:2"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (ImageFile) TableName() string { return "ai_image_files" }

type TaskFileSet struct {
	Inputs []ImageFile
	Mask   *MaskImageFile
}

type MaskImageFile struct {
	File             ImageFile
	RelatedSortOrder int
}

type AgentRuntime struct {
	AgentID                uint64                      `gorm:"column:agent_id"`
	AgentName              string                      `gorm:"column:agent_name"`
	ScenesJSON             string                      `gorm:"column:scenes_json"`
	AgentStatus            int                         `gorm:"column:agent_status"`
	ProviderID             uint64                      `gorm:"column:provider_id"`
	ProviderName           string                      `gorm:"column:provider_name"`
	EngineType             string                      `gorm:"column:engine_type"`
	BaseURL                string                      `gorm:"column:base_url"`
	APIKeyEnc              string                      `gorm:"column:api_key_enc"`
	ProviderStatus         int                         `gorm:"column:provider_status"`
	ModelID                string                      `gorm:"column:model_id"`
	ModelDisplayName       string                      `gorm:"column:model_display_name"`
	ModelStatus            int                         `gorm:"column:model_status"`
	OfficialModelID        string                      `gorm:"column:official_model_id"`
	OfficialCatalogVersion string                      `gorm:"column:official_catalog_version"`
	MappingStatus          officialmodel.MappingStatus `gorm:"column:mapping_status"`
	BillingMultiplierPPM   int64                       `gorm:"column:billing_multiplier_ppm"`
}

type UploadConfig struct {
	SettingID    int64  `gorm:"column:setting_id"`
	Driver       string `gorm:"column:driver"`
	SecretIDEnc  string `gorm:"column:secret_id_enc"`
	SecretKeyEnc string `gorm:"column:secret_key_enc"`
	Bucket       string `gorm:"column:bucket"`
	Region       string `gorm:"column:region"`
	AppID        string `gorm:"column:appid"`
	Endpoint     string `gorm:"column:endpoint"`
	BucketDomain string `gorm:"column:bucket_domain"`
}
