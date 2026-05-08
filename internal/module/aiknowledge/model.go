package aiknowledge

import "time"

type KnowledgeBase struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	Name           string    `gorm:"column:name"`
	Description    *string   `gorm:"column:description"`
	OwnerUserID    int64     `gorm:"column:owner_user_id"`
	Visibility     string    `gorm:"column:visibility"`
	PermissionJSON *string   `gorm:"column:permission_json"`
	ChunkSize      int       `gorm:"column:chunk_size"`
	ChunkOverlap   int       `gorm:"column:chunk_overlap"`
	TopK           int       `gorm:"column:top_k"`
	ScoreThreshold float64   `gorm:"column:score_threshold"`
	Status         int       `gorm:"column:status"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (KnowledgeBase) TableName() string { return "ai_knowledge_bases" }

type Document struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	KnowledgeBaseID int64     `gorm:"column:knowledge_base_id"`
	Title           string    `gorm:"column:title"`
	SourceType      string    `gorm:"column:source_type"`
	Content         string    `gorm:"column:content"`
	ChunkCount      int       `gorm:"column:chunk_count"`
	IndexStatus     int       `gorm:"column:index_status"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (Document) TableName() string { return "ai_knowledge_documents" }

type Chunk struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	KnowledgeBaseID int64     `gorm:"column:knowledge_base_id"`
	DocumentID      int64     `gorm:"column:document_id"`
	ChunkNo         int       `gorm:"column:chunk_no"`
	Content         string    `gorm:"column:content"`
	TokenEstimate   int       `gorm:"column:token_estimate"`
	MetadataJSON    *string   `gorm:"column:metadata_json"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (Chunk) TableName() string { return "ai_knowledge_chunks" }
