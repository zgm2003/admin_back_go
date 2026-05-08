package aiknowledgemap

import "time"

type KnowledgeMap struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	EngineConnectionID uint64    `gorm:"column:engine_connection_id"`
	Name               string    `gorm:"column:name"`
	Code               string    `gorm:"column:code"`
	EngineDatasetID    string    `gorm:"column:engine_dataset_id"`
	Visibility         string    `gorm:"column:visibility"`
	Status             int       `gorm:"column:status"`
	IsDel              int       `gorm:"column:is_del"`
	MetaJSON           string    `gorm:"column:meta_json"`
	CreatedBy          uint64    `gorm:"column:created_by"`
	UpdatedBy          uint64    `gorm:"column:updated_by"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (KnowledgeMap) TableName() string { return "ai_knowledge_maps" }

type Document struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	KnowledgeMapID   uint64    `gorm:"column:knowledge_map_id"`
	Name             string    `gorm:"column:name"`
	EngineDocumentID string    `gorm:"column:engine_document_id"`
	EngineBatch      string    `gorm:"column:engine_batch"`
	SourceType       string    `gorm:"column:source_type"`
	SourceRef        string    `gorm:"column:source_ref"`
	Content          string    `gorm:"column:content"`
	IndexingStatus   string    `gorm:"column:indexing_status"`
	ErrorMessage     string    `gorm:"column:error_message"`
	Status           int       `gorm:"column:status"`
	IsDel            int       `gorm:"column:is_del"`
	MetaJSON         string    `gorm:"column:meta_json"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (Document) TableName() string { return "ai_knowledge_documents" }

type EngineConnection struct {
	ID         uint64 `gorm:"column:id;primaryKey"`
	Name       string `gorm:"column:name"`
	EngineType string `gorm:"column:engine_type"`
	BaseURL    string `gorm:"column:base_url"`
	APIKeyEnc  string `gorm:"column:api_key_enc"`
	Status     int    `gorm:"column:status"`
	IsDel      int    `gorm:"column:is_del"`
}

func (EngineConnection) TableName() string { return "ai_engine_connections" }
