package contextengine

import (
	"fmt"
	"time"

	aiprovider "admin_back_go/internal/module/ai/provider"
)

type ProfileStatus string

const (
	ProfileEnabled        ProfileStatus = "enabled"
	ProfileRetired        ProfileStatus = "retired"
	SpaceEnabled          string        = "enabled"
	SpaceDisabled         string        = "disabled"
	DocumentEnabled       string        = "enabled"
	DocumentDisabled      string        = "disabled"
	DocumentVersionQueued string        = "queued"
	DocumentVersionReady  string        = "ready"
)

func ValidateContextAdminState(resource string, state string) error {
	switch resource {
	case "profile":
		if state == string(ProfileEnabled) || state == string(ProfileRetired) {
			return nil
		}
	case "space":
		if state == SpaceEnabled || state == SpaceDisabled {
			return nil
		}
	case "document":
		if state == DocumentEnabled || state == DocumentDisabled {
			return nil
		}
	case "document_version":
		if state == DocumentVersionQueued || state == DocumentVersionReady || state == DocumentVersionFailed {
			return nil
		}
	}
	return fmt.Errorf("invalid %s state %q", resource, state)
}

type ContextProfile struct {
	ID                       uint64            `json:"id" gorm:"column:id;primaryKey"`
	Name                     string            `json:"name" gorm:"column:name"`
	EmbeddingProviderModelID uint64            `json:"embedding_provider_model_id" gorm:"column:embedding_provider_model_id"`
	EmbeddingDimensions      uint32            `json:"embedding_dimensions" gorm:"column:embedding_dimensions"`
	EmbeddingMaxInputTokens  int64             `json:"embedding_max_input_tokens" gorm:"column:embedding_max_input_tokens"`
	EmbeddingTokenCounterID  string            `json:"embedding_token_counter_id" gorm:"column:embedding_token_counter_id"`
	DenseDistance            string            `json:"dense_distance" gorm:"column:dense_distance"`
	DenseMinScore            string            `json:"dense_min_score" gorm:"column:dense_min_score"`
	SparseEncoder            string            `json:"sparse_encoder" gorm:"column:sparse_encoder"`
	SparseEncoderVersion     string            `json:"sparse_encoder_version" gorm:"column:sparse_encoder_version"`
	RerankerProviderModelID  *uint64           `json:"reranker_provider_model_id" gorm:"column:reranker_provider_model_id"`
	RerankerMinScore         *string           `json:"reranker_min_score" gorm:"column:reranker_min_score"`
	MemoryProviderModelID    *uint64           `json:"memory_provider_model_id" gorm:"column:memory_provider_model_id"`
	Status                   ProfileStatus     `json:"status" gorm:"column:status"`
	ActiveIndexGeneration    *uint64           `json:"active_index_generation" gorm:"column:active_index_generation"`
	TargetIndexGeneration    *uint64           `json:"target_index_generation" gorm:"column:target_index_generation"`
	IndexState               ProfileIndexState `json:"index_state" gorm:"column:index_state"`
	IndexErrorCode           *string           `json:"index_error_code" gorm:"column:index_error_code"`
	IndexVerifiedAt          *time.Time        `json:"index_verified_at" gorm:"column:index_verified_at"`
	CreatedBy                uint32            `json:"created_by" gorm:"column:created_by"`
	CreatedAt                time.Time         `json:"created_at" gorm:"column:created_at"`
	UpdatedAt                time.Time         `json:"updated_at" gorm:"column:updated_at"`
}

func (ContextProfile) TableName() string { return "ai_context_profiles" }

type ContextSpace struct {
	ID          uint64     `json:"id" gorm:"column:id;primaryKey"`
	Platform    string     `json:"platform" gorm:"column:platform"`
	ProfileID   uint64     `json:"profile_id" gorm:"column:profile_id"`
	Name        string     `json:"name" gorm:"column:name"`
	Description string     `json:"description" gorm:"column:description"`
	Status      string     `json:"status" gorm:"column:status"`
	DeletedAt   *time.Time `json:"-" gorm:"column:deleted_at"`
	CreatedBy   uint32     `json:"created_by" gorm:"column:created_by"`
	CreatedAt   time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (ContextSpace) TableName() string { return "ai_context_spaces" }

type ContextDocument struct {
	ID                    uint64     `gorm:"column:id;primaryKey"`
	SpaceID               *uint64    `gorm:"column:space_id"`
	ConversationID        *uint64    `gorm:"column:conversation_id"`
	SourceMessageID       *uint64    `gorm:"column:source_message_id"`
	SourceAttachmentIndex *uint32    `gorm:"column:source_attachment_index"`
	Title                 string     `gorm:"column:title"`
	ActiveVersionID       *uint64    `gorm:"column:active_version_id"`
	Status                string     `gorm:"column:status"`
	DeletedAt             *time.Time `gorm:"column:deleted_at"`
	CreatedBy             uint32     `gorm:"column:created_by"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (ContextDocument) TableName() string { return "ai_context_documents" }

type ContextDocumentVersion struct {
	ID                    uint64    `gorm:"column:id;primaryKey"`
	DocumentID            uint64    `gorm:"column:document_id"`
	ProfileID             uint64    `gorm:"column:profile_id"`
	SourceStorageProvider string    `gorm:"column:source_storage_provider"`
	SourceObjectKey       string    `gorm:"column:source_object_key"`
	SourceETag            string    `gorm:"column:source_etag"`
	SourceSize            int64     `gorm:"column:source_size_bytes"`
	SourceMIMEType        string    `gorm:"column:source_mime_type"`
	SourceFilename        string    `gorm:"column:source_filename"`
	SourceFactsSHA256     []byte    `gorm:"column:source_facts_sha256"`
	ParserName            string    `gorm:"column:parser_name"`
	ParserVersion         string    `gorm:"column:parser_version"`
	ChunkerVersion        string    `gorm:"column:chunker_version"`
	State                 string    `gorm:"column:state"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (ContextDocumentVersion) TableName() string { return "ai_context_document_versions" }

type ProfileDTO = ContextProfile
type SpaceDTO = ContextSpace

type DocumentVersionDTO struct {
	ID                    uint64 `json:"id"`
	DocumentID            uint64 `json:"document_id"`
	ProfileID             uint64 `json:"profile_id"`
	SourceStorageProvider string `json:"source_storage_provider"`
	SourceObjectKey       string `json:"source_object_key"`
	SourceETag            string `json:"source_etag"`
	SourceSize            int64  `json:"source_size_bytes"`
	SourceMIMEType        string `json:"source_mime_type"`
	SourceFilename        string `json:"source_filename"`
	ParserName            string `json:"parser_name"`
	ParserVersion         string `json:"parser_version"`
	ChunkerVersion        string `json:"chunker_version"`
	State                 string `json:"state"`
}

type DocumentAdminDTO struct {
	ID                    uint64             `json:"id"`
	SpaceID               *uint64            `json:"space_id,omitempty"`
	ConversationID        *uint64            `json:"conversation_id,omitempty"`
	SourceMessageID       *uint64            `json:"source_message_id,omitempty"`
	SourceAttachmentIndex *uint32            `json:"source_attachment_index,omitempty"`
	ProfileID             uint64             `json:"profile_id"`
	Title                 string             `json:"title"`
	Status                string             `json:"status"`
	ActiveVersionID       *uint64            `json:"active_version_id,omitempty"`
	Version               DocumentVersionDTO `json:"version"`
}

type ProfileListResponse struct {
	Items []ProfileDTO `json:"items"`
}
type SpaceListResponse struct {
	Items []SpaceDTO `json:"items"`
}
type DocumentListResponse struct {
	Items []DocumentAdminDTO `json:"items"`
}
type DocumentVersionListResponse struct {
	Items []DocumentVersionDTO `json:"items"`
}

type DocumentPreviewKind string

const (
	DocumentPreviewText     DocumentPreviewKind = "text"
	DocumentPreviewMarkdown DocumentPreviewKind = "markdown"
	DocumentPreviewPDF      DocumentPreviewKind = "pdf"
	DocumentPreviewExternal DocumentPreviewKind = "external"
)

type DocumentVersionPreviewResponse struct {
	URL         string              `json:"url"`
	ExpiresIn   int64               `json:"expires_in"`
	Filename    string              `json:"filename"`
	MIMEType    string              `json:"mime_type"`
	SizeBytes   int64               `json:"size_bytes"`
	PreviewKind DocumentPreviewKind `json:"preview_kind" validate:"oneof=text markdown pdf external"`
}

type ProviderModelOption struct {
	ID                      uint64               `gorm:"column:id"`
	ProviderName            string               `gorm:"column:provider_name"`
	ModelID                 string               `gorm:"column:model_id"`
	ModelKind               aiprovider.ModelKind `gorm:"column:model_kind"`
	DisplayName             string               `gorm:"column:display_name"`
	EmbeddingDimensions     *uint32              `gorm:"column:embedding_dimensions"`
	EmbeddingMaxInputTokens *int64               `gorm:"column:embedding_max_input_tokens"`
	EmbeddingTokenCounterID *string              `gorm:"column:embedding_token_counter_id"`
}

type ProviderModelOptionDTO struct {
	Value                   uint64  `json:"value"`
	Label                   string  `json:"label"`
	ProviderName            string  `json:"provider_name"`
	ModelID                 string  `json:"model_id"`
	EmbeddingDimensions     *uint32 `json:"embedding_dimensions,omitempty"`
	EmbeddingMaxInputTokens *int64  `json:"embedding_max_input_tokens,omitempty"`
	EmbeddingTokenCounterID *string `json:"embedding_token_counter_id,omitempty"`
}

type ContextPageInitResponse struct {
	EmbeddingModelOptions []ProviderModelOptionDTO `json:"embedding_model_options"`
	RerankerModelOptions  []ProviderModelOptionDTO `json:"reranker_model_options"`
	MemoryModelOptions    []ProviderModelOptionDTO `json:"memory_model_options"`
}

type ContextProfileStatusInput struct {
	Status ProfileStatus `json:"status" binding:"required"`
}
type ContextSpaceStatusInput struct {
	Status string `json:"status" binding:"required"`
}
type ContextDocumentStatusInput struct {
	Status string `json:"status" binding:"required"`
}

type CreateDocumentVersionInput struct {
	SourceStorageProvider string `json:"source_storage_provider" binding:"required"`
	SourceObjectKey       string `json:"source_object_key" binding:"required"`
	SourceETag            string `json:"source_etag" binding:"required"`
	SourceSize            int64  `json:"source_size_bytes" binding:"required,gt=0"`
	SourceFilename        string `json:"source_filename" binding:"required"`
}

type AgentContextProfileInput struct {
	ProfileID *uint64 `json:"profile_id"`
}
type AgentContextSpacesInput struct {
	SpaceIDs []uint64 `json:"space_ids"`
}
type EvaluationOptions struct{ Persist bool }

type EvaluationItemDTO struct {
	Ordinal         uint32                 `json:"ordinal"`
	Decision        Decision               `json:"decision"`
	SourceType      string                 `json:"source_type"`
	SourceRef       string                 `json:"source_ref"`
	CitationKey     *string                `json:"citation_key,omitempty"`
	ExclusionReason *ExclusionReason       `json:"exclusion_reason,omitempty"`
	FusionScore     *FixedScore            `json:"fusion_score,omitempty"`
	RerankScore     *FixedScore            `json:"rerank_score,omitempty"`
	TokenUpperBound int64                  `json:"token_upper_bound"`
	Metadata        ContextBlockMetadataV1 `json:"metadata"`
}

type ContextEvaluationResponse struct {
	RetrievalOutcome RetrievalOutcome     `json:"retrieval_outcome"`
	Budget           Budget               `json:"budget"`
	Metrics          ContextPlanMetricsV1 `json:"metrics"`
	Selected         []EvaluationItemDTO  `json:"selected"`
	Excluded         []EvaluationItemDTO  `json:"excluded"`
}

type CreateProfileInput struct {
	Name                     string
	EmbeddingProviderModelID uint64
	EmbeddingDimensions      uint32
	EmbeddingMaxInputTokens  int64
	EmbeddingTokenCounterID  string
	DenseDistance            string
	DenseMinScore            string
	RerankerProviderModelID  *uint64
	RerankerMinScore         *string
	MemoryProviderModelID    *uint64
}
type UpdateProfileInput struct {
	Name   string
	Status ProfileStatus
}
type CreateSpaceInput struct {
	ProfileID   uint64
	Name        string
	Description string
	Status      string
}
type UpdateSpaceInput = CreateSpaceInput
type CreateDocumentInput struct {
	SpaceID               *uint64
	ConversationID        *uint64
	SourceMessageID       *uint64
	SourceAttachmentIndex *uint32
	Title                 string
	SourceStorageProvider string
	SourceObjectKey       string
	SourceETag            string
	SourceSize            int64
	SourceFilename        string
}

type ProviderModelCapability struct {
	ID                      uint64
	Kind                    aiprovider.ModelKind
	Enabled                 bool
	ProviderEnabled         bool
	OfficialModelID         string
	EmbeddingDimensions     *uint32
	EmbeddingMaxInputTokens *int64
	EmbeddingTokenCounterID *string
}

type ProfileIndexCAS struct {
	ID       uint64
	Expected ProfileIndex
	Next     ProfileIndex
}

func documentVersionDTO(version ContextDocumentVersion) DocumentVersionDTO {
	return DocumentVersionDTO{ID: version.ID, DocumentID: version.DocumentID, ProfileID: version.ProfileID,
		SourceStorageProvider: version.SourceStorageProvider, SourceObjectKey: version.SourceObjectKey, SourceETag: version.SourceETag,
		SourceSize: version.SourceSize, SourceMIMEType: version.SourceMIMEType, SourceFilename: version.SourceFilename,
		ParserName: version.ParserName, ParserVersion: version.ParserVersion, ChunkerVersion: version.ChunkerVersion, State: version.State}
}

func cloneUint32(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func documentAdminDTO(document ContextDocument, version ContextDocumentVersion) DocumentAdminDTO {
	return DocumentAdminDTO{ID: document.ID, SpaceID: document.SpaceID, ConversationID: document.ConversationID,
		SourceMessageID: document.SourceMessageID, SourceAttachmentIndex: document.SourceAttachmentIndex, ProfileID: version.ProfileID,
		Title: document.Title, Status: document.Status, ActiveVersionID: document.ActiveVersionID, Version: documentVersionDTO(version)}
}
