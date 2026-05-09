package aitoolmap

import "time"

type ToolMap struct {
	ID             uint64    `gorm:"column:id;primaryKey"`
	ProviderID     uint64    `gorm:"column:provider_id"`
	AppID          *uint64   `gorm:"column:app_id"`
	Name           string    `gorm:"column:name"`
	Code           string    `gorm:"column:code"`
	ToolType       string    `gorm:"column:tool_type"`
	EngineToolID   string    `gorm:"column:engine_tool_id"`
	PermissionCode string    `gorm:"column:permission_code"`
	RiskLevel      string    `gorm:"column:risk_level"`
	ConfigJSON     string    `gorm:"column:config_json"`
	Status         int       `gorm:"column:status"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedBy      uint64    `gorm:"column:created_by"`
	UpdatedBy      uint64    `gorm:"column:updated_by"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (ToolMap) TableName() string { return "ai_tool_maps" }

type Provider struct {
	ID         uint64 `gorm:"column:id;primaryKey"`
	Name       string `gorm:"column:name"`
	EngineType string `gorm:"column:engine_type"`
	BaseURL    string `gorm:"column:base_url"`
	APIKeyEnc  string `gorm:"column:api_key_enc"`
	Status     int    `gorm:"column:status"`
	IsDel      int    `gorm:"column:is_del"`
}

func (Provider) TableName() string { return "ai_providers" }
