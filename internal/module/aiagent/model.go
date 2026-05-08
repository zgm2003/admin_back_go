package aiagent

import "time"

type Agent struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	Name              string    `gorm:"column:name"`
	ModelID           int64     `gorm:"column:model_id"`
	Avatar            *string   `gorm:"column:avatar"`
	SystemPrompt      *string   `gorm:"column:system_prompt"`
	Mode              string    `gorm:"column:mode"`
	Scene             *string   `gorm:"column:scene"`
	CapabilitiesJSON  *string   `gorm:"column:capabilities_json"`
	RuntimeConfigJSON *string   `gorm:"column:runtime_config_json"`
	PolicyJSON        *string   `gorm:"column:policy_json"`
	Status            int       `gorm:"column:status"`
	IsDel             int       `gorm:"column:is_del"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (Agent) TableName() string { return "ai_agents" }

type AgentScene struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	AgentID   int64     `gorm:"column:agent_id"`
	SceneCode string    `gorm:"column:scene_code"`
	Status    int       `gorm:"column:status"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (AgentScene) TableName() string { return "ai_agent_scenes" }

type AssistantTool struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	AssistantID int64     `gorm:"column:assistant_id"`
	ToolID      int64     `gorm:"column:tool_id"`
	Status      int       `gorm:"column:status"`
	IsDel       int       `gorm:"column:is_del"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (AssistantTool) TableName() string { return "ai_assistant_tools" }

type AgentKnowledgeBase struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	AgentID         int64     `gorm:"column:agent_id"`
	KnowledgeBaseID int64     `gorm:"column:knowledge_base_id"`
	Status          int       `gorm:"column:status"`
	IsDel           int       `gorm:"column:is_del"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (AgentKnowledgeBase) TableName() string { return "ai_agent_knowledge_bases" }
