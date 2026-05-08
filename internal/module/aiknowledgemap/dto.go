package aiknowledgemap

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	platformai "admin_back_go/internal/platform/ai"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	VisibilityArr           []dict.Option[string] `json:"visibility_arr"`
	SourceTypeArr           []dict.Option[string] `json:"source_type_arr"`
	IndexingStatusArr       []dict.Option[string] `json:"indexing_status_arr"`
	CommonStatusArr         []dict.Option[int]    `json:"common_status_arr"`
	EngineConnectionOptions []EngineOption        `json:"engine_connection_options"`
}

type EngineOption struct {
	Label      string `json:"label"`
	Value      uint64 `json:"value"`
	EngineType string `json:"engine_type"`
}

type ListQuery struct {
	CurrentPage        int
	PageSize           int
	Name               string
	Code               string
	Visibility         string
	EngineConnectionID uint64
	Status             *int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []MapDTO `json:"list"`
	Page Page     `json:"page"`
}

type DetailResponse struct {
	MapDTO
}

type MapDTO struct {
	ID                   uint64          `json:"id"`
	EngineConnectionID   uint64          `json:"engine_connection_id"`
	EngineConnectionName string          `json:"engine_connection_name"`
	EngineType           string          `json:"engine_type"`
	Name                 string          `json:"name"`
	Code                 string          `json:"code"`
	EngineDatasetID      string          `json:"engine_dataset_id"`
	Visibility           string          `json:"visibility"`
	VisibilityName       string          `json:"visibility_name"`
	MetaJSON             json.RawMessage `json:"meta_json"`
	Status               int             `json:"status"`
	StatusName           string          `json:"status_name"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

type MapInput struct {
	EngineConnectionID uint64
	Name               string
	Code               string
	EngineDatasetID    string
	Visibility         string
	MetaJSON           json.RawMessage
	Status             int
}

type DocumentInput struct {
	Name       string
	SourceType string
	SourceRef  string
	Content    string
	MetaJSON   json.RawMessage
	Status     int
}

type DocumentListResponse struct {
	List []DocumentDTO `json:"list"`
}

type DocumentDTO struct {
	ID               uint64          `json:"id"`
	KnowledgeMapID   uint64          `json:"knowledge_map_id"`
	Name             string          `json:"name"`
	EngineDocumentID string          `json:"engine_document_id"`
	EngineBatch      string          `json:"engine_batch"`
	SourceType       string          `json:"source_type"`
	SourceTypeName   string          `json:"source_type_name"`
	SourceRef        string          `json:"source_ref"`
	IndexingStatus   string          `json:"indexing_status"`
	ErrorMessage     string          `json:"error_message"`
	MetaJSON         json.RawMessage `json:"meta_json"`
	Status           int             `json:"status"`
	StatusName       string          `json:"status_name"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

type EngineConfig struct {
	EngineType platformai.EngineType
	BaseURL    string
	APIKey     string
}

type EngineFactory interface {
	NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error)
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error)
	Create(ctx context.Context, input MapInput) (uint64, *apperror.Error)
	Update(ctx context.Context, id uint64, input MapInput) *apperror.Error
	ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error
	Sync(ctx context.Context, id uint64) *apperror.Error
	Delete(ctx context.Context, id uint64) *apperror.Error
	Documents(ctx context.Context, mapID uint64) (*DocumentListResponse, *apperror.Error)
	CreateDocument(ctx context.Context, mapID uint64, input DocumentInput) (uint64, *apperror.Error)
	ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error
	RefreshDocumentStatus(ctx context.Context, id uint64) *apperror.Error
	DeleteDocument(ctx context.Context, id uint64) *apperror.Error
}
