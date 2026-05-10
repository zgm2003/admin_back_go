package aiknowledge

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type ChunkOptions struct {
	SizeChars    uint
	OverlapChars uint
}

type TextChunk struct {
	Index   uint
	Content string
	Chars   uint
}

type RetrievalOptions struct {
	TopK            uint
	MinScore        float64
	MaxContextChars uint
}

type RetrievalCandidate struct {
	KnowledgeBaseID   uint64
	KnowledgeBaseName string
	DocumentID        uint64
	DocumentTitle     string
	ChunkID           uint64
	ChunkIndex        uint
	Title             string
	Content           string
	ContentChars      uint
}

type RetrievalResult struct {
	Query        string         `json:"query"`
	Status       string         `json:"status"`
	TotalHits    uint           `json:"total_hits"`
	SelectedHits uint           `json:"selected_hits"`
	Hits         []RetrievalHit `json:"hits"`
	Selected     []SelectedHit  `json:"selected"`
}

type RetrievalHit struct {
	KnowledgeBaseID   uint64  `json:"knowledge_base_id"`
	KnowledgeBaseName string  `json:"knowledge_base_name"`
	DocumentID        uint64  `json:"document_id"`
	DocumentTitle     string  `json:"document_title"`
	ChunkID           uint64  `json:"chunk_id"`
	ChunkIndex        uint    `json:"chunk_index"`
	Score             float64 `json:"score"`
	RankNo            uint    `json:"rank_no"`
	Content           string  `json:"content"`
	ContentChars      uint    `json:"content_chars"`
	Status            int     `json:"status"`
	SkipReason        string  `json:"skip_reason"`
}

type SelectedHit struct {
	Ref               string  `json:"ref"`
	KnowledgeBaseID   uint64  `json:"knowledge_base_id"`
	KnowledgeBaseName string  `json:"knowledge_base_name"`
	DocumentID        uint64  `json:"document_id"`
	DocumentTitle     string  `json:"document_title"`
	ChunkID           uint64  `json:"chunk_id"`
	ChunkIndex        uint    `json:"chunk_index"`
	Score             float64 `json:"score"`
	RankNo            uint    `json:"rank_no"`
	Content           string  `json:"content"`
}

type ScoredHit = RetrievalHit

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	CommonStatusArr []dict.Option[int]    `json:"common_status_arr"`
	SourceTypeArr   []dict.Option[string] `json:"source_type_arr"`
	IndexStatusArr  []dict.Option[string] `json:"index_status_arr"`
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type BaseListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Code        string
	Status      *int
}

type BaseListResponse struct {
	List []BaseDTO `json:"list"`
	Page Page      `json:"page"`
}

type BaseDetailResponse struct {
	BaseDTO
}

type BaseDTO struct {
	ID                     uint64  `json:"id"`
	Name                   string  `json:"name"`
	Code                   string  `json:"code"`
	Description            string  `json:"description"`
	ChunkSizeChars         uint    `json:"chunk_size_chars"`
	ChunkOverlapChars      uint    `json:"chunk_overlap_chars"`
	DefaultTopK            uint    `json:"default_top_k"`
	DefaultMinScore        float64 `json:"default_min_score"`
	DefaultMaxContextChars uint    `json:"default_max_context_chars"`
	Status                 int     `json:"status"`
	StatusName             string  `json:"status_name"`
	CreatedAt              string  `json:"created_at"`
	UpdatedAt              string  `json:"updated_at"`
}

type BaseMutationInput struct {
	Name                   string
	Code                   string
	Description            string
	ChunkSizeChars         uint
	ChunkOverlapChars      uint
	DefaultTopK            uint
	DefaultMinScore        float64
	DefaultMaxContextChars uint
	Status                 int
}

type DocumentListQuery struct {
	CurrentPage int
	PageSize    int
	Title       string
	Status      *int
}

type DocumentListResponse struct {
	List []DocumentDTO `json:"list"`
	Page Page          `json:"page"`
}

type DocumentDetailResponse struct {
	DocumentDTO
	Content string `json:"content"`
}

type DocumentDTO struct {
	ID              uint64 `json:"id"`
	KnowledgeBaseID uint64 `json:"knowledge_base_id"`
	Title           string `json:"title"`
	SourceType      string `json:"source_type"`
	SourceTypeName  string `json:"source_type_name"`
	SourceRef       string `json:"source_ref"`
	IndexStatus     string `json:"index_status"`
	IndexStatusName string `json:"index_status_name"`
	ErrorMessage    string `json:"error_message"`
	LastIndexedAt   string `json:"last_indexed_at"`
	Status          int    `json:"status"`
	StatusName      string `json:"status_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type DocumentMutationInput struct {
	Title      string
	SourceType string
	SourceRef  string
	Content    string
	Status     int
}

type ChunkListResponse struct {
	List []ChunkDTO `json:"list"`
}

type ChunkDTO struct {
	ID              uint64 `json:"id"`
	KnowledgeBaseID uint64 `json:"knowledge_base_id"`
	DocumentID      uint64 `json:"document_id"`
	ChunkIndex      uint   `json:"chunk_index"`
	Title           string `json:"title"`
	Content         string `json:"content"`
	ContentChars    uint   `json:"content_chars"`
	Status          int    `json:"status"`
	StatusName      string `json:"status_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type RetrievalTestInput struct {
	Query           string
	TopK            uint
	MinScore        *float64
	MaxContextChars uint
}

type KnowledgeBaseOptionRow struct {
	ID                     uint64
	Name                   string
	Description            string
	DefaultTopK            uint
	DefaultMinScore        float64
	DefaultMaxContextChars uint
}

type KnowledgeBaseOption struct {
	Label                  string  `json:"label"`
	Value                  uint64  `json:"value"`
	Description            string  `json:"description"`
	DefaultTopK            uint    `json:"default_top_k"`
	DefaultMinScore        float64 `json:"default_min_score"`
	DefaultMaxContextChars uint    `json:"default_max_context_chars"`
}

type AgentKnowledgeBindingRow struct {
	ID                     uint64
	AgentID                uint64
	KnowledgeBaseID        uint64
	KnowledgeBaseName      string
	TopK                   uint
	MinScore               float64
	MaxContextChars        uint
	Status                 int
	DefaultTopK            uint
	DefaultMinScore        float64
	DefaultMaxContextChars uint
}

type AgentKnowledgeBindingInput struct {
	KnowledgeBaseID uint64   `json:"knowledge_base_id"`
	TopK            uint     `json:"top_k"`
	MinScore        *float64 `json:"min_score"`
	MaxContextChars uint     `json:"max_context_chars"`
	Status          int      `json:"status"`
}

type UpdateAgentKnowledgeBindingsInput struct {
	Bindings []AgentKnowledgeBindingInput `json:"bindings"`
}

type AgentKnowledgeBindingsResponse struct {
	AgentID     uint64                      `json:"agent_id"`
	Bindings    []AgentKnowledgeBindingItem `json:"bindings"`
	BaseOptions []KnowledgeBaseOption       `json:"base_options"`
}

type AgentKnowledgeBindingItem struct {
	ID                uint64  `json:"id,omitempty"`
	KnowledgeBaseID   uint64  `json:"knowledge_base_id"`
	KnowledgeBaseName string  `json:"knowledge_base_name"`
	TopK              uint    `json:"top_k"`
	MinScore          float64 `json:"min_score"`
	MaxContextChars   uint    `json:"max_context_chars"`
	Status            int     `json:"status"`
	StatusName        string  `json:"status_name"`
}

type RuntimeBindingRow struct {
	KnowledgeBaseID   uint64
	KnowledgeBaseName string
	TopK              uint
	MinScore          float64
	MaxContextChars   uint
}

type KnowledgeRuntimeInput struct {
	RunID          uint64
	AgentID        uint64
	ConversationID int64
	UserMessageID  int64
	Query          string
	StartedAt      time.Time
}

type KnowledgeContextResult struct {
	RetrievalID uint64
	Status      string
	Context     string
}

type CreateRetrievalInput struct {
	RunID     uint64
	Query     string
	Status    string
	StartedAt time.Time
}

type FinishRetrievalInput struct {
	ID           uint64
	Status       string
	TotalHits    uint
	SelectedHits uint
	DurationMS   uint
	ErrorMessage string
}

type Repository interface {
	ListBases(ctx context.Context, query BaseListQuery) ([]KnowledgeBase, int64, error)
	GetBase(ctx context.Context, id uint64) (*KnowledgeBase, error)
	CreateBase(ctx context.Context, row KnowledgeBase) (uint64, error)
	UpdateBase(ctx context.Context, id uint64, fields map[string]any) error
	ChangeBaseStatus(ctx context.Context, id uint64, status int) error
	DeleteBase(ctx context.Context, id uint64) error
	ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) ([]KnowledgeDocument, int64, error)
	GetDocument(ctx context.Context, id uint64) (*KnowledgeDocument, error)
	CreateDocument(ctx context.Context, row KnowledgeDocument) (uint64, error)
	UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error
	ChangeDocumentStatus(ctx context.Context, id uint64, status int) error
	DeleteDocument(ctx context.Context, id uint64) error
	ReplaceChunks(ctx context.Context, document KnowledgeDocument, chunks []TextChunk, indexedAt time.Time) error
	ListChunks(ctx context.Context, documentID uint64) ([]KnowledgeChunk, error)
	ListActiveBaseOptions(ctx context.Context) ([]KnowledgeBaseOptionRow, error)
	ListAgentKnowledgeBindings(ctx context.Context, agentID uint64) ([]AgentKnowledgeBindingRow, error)
	ReplaceAgentKnowledgeBindings(ctx context.Context, agentID uint64, rows []AgentKnowledgeBindingInput) error
	ListRuntimeBindings(ctx context.Context, agentID uint64) ([]RuntimeBindingRow, error)
	ListCandidates(ctx context.Context, baseIDs []uint64, limit int) ([]RetrievalCandidate, error)
	CreateRetrieval(ctx context.Context, input CreateRetrievalInput) (uint64, error)
	FinishRetrieval(ctx context.Context, input FinishRetrievalInput) error
	InsertRetrievalHits(ctx context.Context, retrievalID uint64, hits []ScoredHit) error
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	ListBases(ctx context.Context, query BaseListQuery) (*BaseListResponse, *apperror.Error)
	GetBase(ctx context.Context, id uint64) (*BaseDetailResponse, *apperror.Error)
	CreateBase(ctx context.Context, input BaseMutationInput) (uint64, *apperror.Error)
	UpdateBase(ctx context.Context, id uint64, input BaseMutationInput) *apperror.Error
	ChangeBaseStatus(ctx context.Context, id uint64, status int) *apperror.Error
	DeleteBase(ctx context.Context, id uint64) *apperror.Error
	ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) (*DocumentListResponse, *apperror.Error)
	GetDocument(ctx context.Context, id uint64) (*DocumentDetailResponse, *apperror.Error)
	CreateDocument(ctx context.Context, baseID uint64, input DocumentMutationInput) (uint64, *apperror.Error)
	UpdateDocument(ctx context.Context, id uint64, input DocumentMutationInput) *apperror.Error
	ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error
	DeleteDocument(ctx context.Context, id uint64) *apperror.Error
	ReindexDocument(ctx context.Context, id uint64) *apperror.Error
	ListChunks(ctx context.Context, documentID uint64) (*ChunkListResponse, *apperror.Error)
	RetrievalTest(ctx context.Context, baseID uint64, input RetrievalTestInput) (*RetrievalResult, *apperror.Error)
	AgentKnowledgeBases(ctx context.Context, agentID uint64) (*AgentKnowledgeBindingsResponse, *apperror.Error)
	UpdateAgentKnowledgeBases(ctx context.Context, agentID uint64, input UpdateAgentKnowledgeBindingsInput) *apperror.Error
}

type RuntimeService interface {
	RetrieveForRun(ctx context.Context, input KnowledgeRuntimeInput) (*KnowledgeContextResult, *apperror.Error)
}
