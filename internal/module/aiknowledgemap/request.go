package aiknowledgemap

import "encoding/json"

type listRequest struct {
	CurrentPage int    `form:"current_page" binding:"omitempty,min=1"`
	PageSize    int    `form:"page_size" binding:"omitempty,min=1,max=50"`
	Name        string `form:"name" binding:"max=128"`
	Code        string `form:"code" binding:"max=128"`
	Visibility  string `form:"visibility" binding:"omitempty,oneof=private public"`
	ProviderID  uint64 `form:"provider_id" binding:"omitempty,gt=0"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type mapRequest struct {
	ProviderID      uint64          `json:"provider_id" binding:"required,gt=0"`
	Name            string          `json:"name" binding:"required,max=128"`
	Code            string          `json:"code" binding:"required,max=128"`
	EngineDatasetID string          `json:"engine_dataset_id" binding:"omitempty,max=128"`
	Visibility      string          `json:"visibility" binding:"required,oneof=private public"`
	MetaJSON        json.RawMessage `json:"meta_json"`
	Status          int             `json:"status" binding:"required,oneof=1 2"`
}

type documentRequest struct {
	Name       string          `json:"name" binding:"required,max=255"`
	SourceType string          `json:"source_type" binding:"required,oneof=text file"`
	SourceRef  string          `json:"source_ref" binding:"omitempty,max=512"`
	Content    string          `json:"content"`
	MetaJSON   json.RawMessage `json:"meta_json"`
	Status     int             `json:"status" binding:"required,oneof=1 2"`
}

type statusRequest struct {
	Status int `json:"status" binding:"required,oneof=1 2"`
}
