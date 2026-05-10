package aiknowledge

import "time"

type KnowledgeBase struct {
	ID                     uint64    `gorm:"column:id;primaryKey"`
	Name                   string    `gorm:"column:name"`
	Code                   string    `gorm:"column:code"`
	Description            string    `gorm:"column:description"`
	ChunkSizeChars         uint      `gorm:"column:chunk_size_chars"`
	ChunkOverlapChars      uint      `gorm:"column:chunk_overlap_chars"`
	DefaultTopK            uint      `gorm:"column:default_top_k"`
	DefaultMinScore        float64   `gorm:"column:default_min_score"`
	DefaultMaxContextChars uint      `gorm:"column:default_max_context_chars"`
	Status                 int       `gorm:"column:status"`
	IsDel                  int       `gorm:"column:is_del"`
	CreatedAt              time.Time `gorm:"column:created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
}

func (KnowledgeBase) TableName() string { return "ai_knowledge_bases" }

type KnowledgeDocument struct {
	ID              uint64     `gorm:"column:id;primaryKey"`
	KnowledgeBaseID uint64     `gorm:"column:knowledge_base_id"`
	Title           string     `gorm:"column:title"`
	SourceType      string     `gorm:"column:source_type"`
	SourceRef       string     `gorm:"column:source_ref"`
	Content         string     `gorm:"column:content"`
	IndexStatus     string     `gorm:"column:index_status"`
	ErrorMessage    string     `gorm:"column:error_message"`
	LastIndexedAt   *time.Time `gorm:"column:last_indexed_at"`
	Status          int        `gorm:"column:status"`
	IsDel           int        `gorm:"column:is_del"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (KnowledgeDocument) TableName() string { return "ai_knowledge_documents" }

type KnowledgeChunk struct {
	ID              uint64    `gorm:"column:id;primaryKey"`
	KnowledgeBaseID uint64    `gorm:"column:knowledge_base_id"`
	DocumentID      uint64    `gorm:"column:document_id"`
	ChunkIndex      uint      `gorm:"column:chunk_index"`
	Title           string    `gorm:"column:title"`
	Content         string    `gorm:"column:content"`
	ContentChars    uint      `gorm:"column:content_chars"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (KnowledgeChunk) TableName() string { return "ai_knowledge_chunks" }

type AgentKnowledgeBase struct {
	ID              uint64    `gorm:"column:id;primaryKey"`
	AgentID         uint64    `gorm:"column:agent_id"`
	KnowledgeBaseID uint64    `gorm:"column:knowledge_base_id"`
	TopK            uint      `gorm:"column:top_k"`
	MinScore        float64   `gorm:"column:min_score"`
	MaxContextChars uint      `gorm:"column:max_context_chars"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (AgentKnowledgeBase) TableName() string { return "ai_agent_knowledge_bases" }

type KnowledgeRetrieval struct {
	ID           uint64    `gorm:"column:id;primaryKey"`
	RunID        uint64    `gorm:"column:run_id"`
	Query        string    `gorm:"column:query"`
	Status       string    `gorm:"column:status"`
	TotalHits    uint      `gorm:"column:total_hits"`
	SelectedHits uint      `gorm:"column:selected_hits"`
	DurationMS   *uint     `gorm:"column:duration_ms"`
	ErrorMessage string    `gorm:"column:error_message"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (KnowledgeRetrieval) TableName() string { return "ai_knowledge_retrievals" }

type KnowledgeRetrievalHit struct {
	ID                uint64    `gorm:"column:id;primaryKey"`
	RetrievalID       uint64    `gorm:"column:retrieval_id"`
	KnowledgeBaseID   uint64    `gorm:"column:knowledge_base_id"`
	KnowledgeBaseName string    `gorm:"column:knowledge_base_name"`
	DocumentID        uint64    `gorm:"column:document_id"`
	DocumentTitle     string    `gorm:"column:document_title"`
	ChunkID           uint64    `gorm:"column:chunk_id"`
	ChunkIndex        uint      `gorm:"column:chunk_index"`
	Score             float64   `gorm:"column:score"`
	RankNo            uint      `gorm:"column:rank_no"`
	ContentSnapshot   string    `gorm:"column:content_snapshot"`
	Status            int       `gorm:"column:status"`
	SkipReason        string    `gorm:"column:skip_reason"`
	IsDel             int       `gorm:"column:is_del"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (KnowledgeRetrievalHit) TableName() string { return "ai_knowledge_retrieval_hits" }
