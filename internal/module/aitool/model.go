package aitool

import "time"

const retiredCineToolCode = "cine_generate_keyframe"

type Tool struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	Name           string    `gorm:"column:name"`
	Code           string    `gorm:"column:code"`
	Description    string    `gorm:"column:description"`
	SchemaJSON     *string   `gorm:"column:schema_json"`
	ExecutorType   int       `gorm:"column:executor_type"`
	ExecutorConfig *string   `gorm:"column:executor_config"`
	Status         int       `gorm:"column:status"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (Tool) TableName() string {
	return "ai_tools"
}

type AssistantTool struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	AssistantID int64     `gorm:"column:assistant_id"`
	ToolID      int64     `gorm:"column:tool_id"`
	ConfigJSON  *string   `gorm:"column:config_json"`
	Status      int       `gorm:"column:status"`
	IsDel       int       `gorm:"column:is_del"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (AssistantTool) TableName() string {
	return "ai_assistant_tools"
}

type ToolOptionRow struct {
	ID   int64
	Name string
	Code string
}
