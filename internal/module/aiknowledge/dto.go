package aiknowledge

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type JSONObject map[string]any

type InitResponse struct {
	Dict InitDict `json:"dict"`
}
type InitDict struct {
	CommonStatusArr           []dict.Option[int]    `json:"common_status_arr"`
	AIKnowledgeVisibilityArr  []dict.Option[string] `json:"ai_knowledge_visibility_arr"`
	AIKnowledgeIndexStatusArr []dict.Option[int]    `json:"ai_knowledge_index_status_arr"`
	AIKnowledgeSourceTypeArr  []dict.Option[string] `json:"ai_knowledge_source_type_arr"`
}
type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
	Visibility  string
	Status      *int
}
type ListResponse struct {
	List []KnowledgeBaseItem `json:"list"`
	Page Page                `json:"page"`
}
type KnowledgeBaseItem struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Description    *string    `json:"description"`
	OwnerUserID    int64      `json:"owner_user_id"`
	Visibility     string     `json:"visibility"`
	VisibilityName string     `json:"visibility_name"`
	PermissionJSON JSONObject `json:"permission_json"`
	ChunkSize      int        `json:"chunk_size"`
	ChunkOverlap   int        `json:"chunk_overlap"`
	TopK           int        `json:"top_k"`
	ScoreThreshold float64    `json:"score_threshold"`
	Status         int        `json:"status"`
	StatusName     string     `json:"status_name"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}
type KnowledgeBaseMutationInput struct {
	Name           string
	Description    *string
	OwnerUserID    int64
	Visibility     string
	PermissionJSON JSONObject
	ChunkSize      int
	ChunkOverlap   int
	TopK           int
	ScoreThreshold float64
	Status         int
}

type DocumentListQuery struct {
	CurrentPage     int
	PageSize        int
	KnowledgeBaseID int64
	Title           string
	Status          *int
}
type DocumentListResponse struct {
	List []DocumentItem `json:"list"`
	Page Page           `json:"page"`
}
type DocumentItem struct {
	ID              int64  `json:"id"`
	KnowledgeBaseID int64  `json:"knowledge_base_id"`
	Title           string `json:"title"`
	SourceType      string `json:"source_type"`
	SourceTypeName  string `json:"source_type_name"`
	ChunkCount      int    `json:"chunk_count"`
	IndexStatus     int    `json:"index_status"`
	IndexStatusName string `json:"index_status_name"`
	Status          int    `json:"status"`
	StatusName      string `json:"status_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	Content         string `json:"content,omitempty"`
}
type DocumentMutationInput struct {
	KnowledgeBaseID int64
	Title           string
	SourceType      string
	Content         string
	Status          int
}

type ChunkListQuery struct {
	CurrentPage     int
	PageSize        int
	KnowledgeBaseID int64
	DocumentID      *int64
}
type ChunkListResponse struct {
	List []ChunkItem `json:"list"`
	Page Page        `json:"page"`
}
type ChunkItem struct {
	ID              int64      `json:"id"`
	KnowledgeBaseID int64      `json:"knowledge_base_id"`
	DocumentID      int64      `json:"document_id"`
	ChunkNo         int        `json:"chunk_no"`
	Content         string     `json:"content"`
	TokenEstimate   int        `json:"token_estimate"`
	MetadataJSON    JSONObject `json:"metadata_json"`
	Status          int        `json:"status"`
	CreatedAt       string     `json:"created_at"`
}

type RetrievalInput struct {
	KnowledgeBaseID int64
	Query           string
	TopK            int
}
type RetrievalResponse struct {
	Chunks        []RetrievalChunk `json:"chunks"`
	ContextPrompt string           `json:"context_prompt"`
}

type Repository interface {
	Init(ctx context.Context) (*InitResponse, error)
	List(ctx context.Context, query ListQuery) ([]KnowledgeBase, int64, error)
	Get(ctx context.Context, id int64) (*KnowledgeBase, error)
	Create(ctx context.Context, row KnowledgeBase) (int64, error)
	Update(ctx context.Context, id int64, fields map[string]any) error
	ChangeStatus(ctx context.Context, id int64, status int) error
	Delete(ctx context.Context, ids []int64) (int64, error)
	ListDocuments(ctx context.Context, query DocumentListQuery) ([]Document, int64, error)
	GetDocument(ctx context.Context, id int64, knowledgeBaseID int64) (*Document, error)
	CreateDocument(ctx context.Context, row Document) (int64, error)
	UpdateDocument(ctx context.Context, id int64, fields map[string]any) error
	DeleteDocument(ctx context.Context, id int64, knowledgeBaseID int64) error
	ListChunks(ctx context.Context, query ChunkListQuery) ([]Chunk, int64, error)
	ReplaceDocumentChunks(ctx context.Context, knowledgeBaseID int64, documentID int64, chunks []ChunkPayload) (int, error)
	UpdateDocumentChunkStatus(ctx context.Context, id int64, chunkCount int, indexStatus int) error
	CandidateChunks(ctx context.Context, knowledgeBaseID int64, terms []string, limit int) ([]RetrievalChunk, error)
	WithTx(ctx context.Context, fn func(Repository) error) error
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*KnowledgeBaseItem, *apperror.Error)
	Create(ctx context.Context, ownerUserID int64, input KnowledgeBaseMutationInput) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input KnowledgeBaseMutationInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, ids []int64) (int64, *apperror.Error)
	Documents(ctx context.Context, query DocumentListQuery) (*DocumentListResponse, *apperror.Error)
	DocumentDetail(ctx context.Context, id int64, knowledgeBaseID int64) (*DocumentItem, *apperror.Error)
	CreateDocument(ctx context.Context, ownerUserID int64, input DocumentMutationInput) (int64, *apperror.Error)
	UpdateDocument(ctx context.Context, id int64, input DocumentMutationInput) *apperror.Error
	DeleteDocument(ctx context.Context, id int64, knowledgeBaseID int64) (int64, *apperror.Error)
	ReindexDocument(ctx context.Context, id int64, knowledgeBaseID int64) (int, *apperror.Error)
	Chunks(ctx context.Context, query ChunkListQuery) (*ChunkListResponse, *apperror.Error)
	RetrievalTest(ctx context.Context, input RetrievalInput) (*RetrievalResponse, *apperror.Error)
}
