package aitool

import "time"

// Tool maps ai_tools. Every field is used by management, runtime, or audit paths.
type Tool struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	Name             string    `gorm:"column:name"`
	Code             string    `gorm:"column:code"`
	Description      string    `gorm:"column:description"`
	ParametersJSON   string    `gorm:"column:parameters_json"`
	ResultSchemaJSON string    `gorm:"column:result_schema_json"`
	RiskLevel        string    `gorm:"column:risk_level"`
	TimeoutMS        uint      `gorm:"column:timeout_ms"`
	Status           int       `gorm:"column:status"`
	IsDel            int       `gorm:"column:is_del"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (Tool) TableName() string { return "ai_tools" }

// AgentTool maps ai_agent_tools.
type AgentTool struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	AgentID   uint64    `gorm:"column:agent_id"`
	ToolID    uint64    `gorm:"column:tool_id"`
	Status    int       `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (AgentTool) TableName() string { return "ai_agent_tools" }

// ToolCall maps ai_tool_calls.
type ToolCall struct {
	ID            uint64     `gorm:"column:id;primaryKey"`
	RunID         uint64     `gorm:"column:run_id"`
	ToolID        uint64     `gorm:"column:tool_id"`
	ToolCode      string     `gorm:"column:tool_code"`
	ToolName      string     `gorm:"column:tool_name"`
	CallID        *string    `gorm:"column:call_id"`
	Status        string     `gorm:"column:status"`
	ArgumentsJSON string     `gorm:"column:arguments_json"`
	ResultJSON    *string    `gorm:"column:result_json"`
	ErrorMessage  string     `gorm:"column:error_message"`
	DurationMS    *uint      `gorm:"column:duration_ms"`
	StartedAt     time.Time  `gorm:"column:started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (ToolCall) TableName() string { return "ai_tool_calls" }

// Agent is the minimal ai_agents model used to prove a tool binding owner exists.
type Agent struct {
	ID    uint64 `gorm:"column:id;primaryKey"`
	IsDel int    `gorm:"column:is_del"`
}

func (Agent) TableName() string { return "ai_agents" }
