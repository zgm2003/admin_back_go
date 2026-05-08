package aichat

type createRunRequest struct {
	Content        string       `json:"content" binding:"required"`
	ConversationID int64        `json:"conversation_id" binding:"omitempty,min=1"`
	AgentID        int64        `json:"agent_id" binding:"omitempty,min=1"`
	MaxHistory     int          `json:"max_history" binding:"omitempty,min=0,max=100"`
	Attachments    []Attachment `json:"attachments"`
	Temperature    *float64     `json:"temperature" binding:"omitempty,min=0,max=2"`
	MaxTokens      *int         `json:"max_tokens" binding:"omitempty,min=1"`
}

type eventsRequest struct {
	LastID    string `form:"last_id"`
	TimeoutMS int    `form:"timeout_ms" binding:"omitempty,min=0,max=30000"`
}
