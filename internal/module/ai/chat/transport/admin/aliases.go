package admin

import aichatmodule "admin_back_go/internal/module/ai/chat"

type (
	ConversationReplyInput   = aichatmodule.ConversationReplyInput
	ConversationReplyResult  = aichatmodule.ConversationReplyResult
	RunTimeoutInput          = aichatmodule.RunTimeoutInput
	RunTimeoutResult         = aichatmodule.RunTimeoutResult
	CreateRunRecord          = aichatmodule.CreateRunRecord
	CompleteRunRecord        = aichatmodule.CompleteRunRecord
	FinishRunRecord          = aichatmodule.FinishRunRecord
	AssistantMessageRecord   = aichatmodule.AssistantMessageRecord
	EngineConfig             = aichatmodule.EngineConfig
	EngineFactory            = aichatmodule.EngineFactory
	AgentEngineConfig        = aichatmodule.AgentEngineConfig
	MessageHistory           = aichatmodule.MessageHistory
	RuntimeTool              = aichatmodule.RuntimeTool
	ToolRuntime              = aichatmodule.ToolRuntime
	ToolExecuteInput         = aichatmodule.ToolExecuteInput
	ToolExecuteResult        = aichatmodule.ToolExecuteResult
	KnowledgeRuntime         = aichatmodule.KnowledgeRuntime
	KnowledgeRuntimeInput    = aichatmodule.KnowledgeRuntimeInput
	KnowledgeContextResult   = aichatmodule.KnowledgeContextResult
	Repository               = aichatmodule.Repository
	HTTPService              = aichatmodule.HTTPService
	JobService               = aichatmodule.JobService
	EnvelopeEvent            = aichatmodule.EnvelopeEvent
	StreamIDGenerator        = aichatmodule.StreamIDGenerator
	StartPayload             = aichatmodule.StartPayload
	DeltaPayload             = aichatmodule.DeltaPayload
	CompletedPayload         = aichatmodule.CompletedPayload
	FailedPayload            = aichatmodule.FailedPayload
	ConversationReplyPayload = aichatmodule.ConversationReplyPayload
	RunTimeoutPayload        = aichatmodule.RunTimeoutPayload
	Conversation             = aichatmodule.Conversation
	Message                  = aichatmodule.Message
	Run                      = aichatmodule.Run
	RunEvent                 = aichatmodule.RunEvent
	Agent                    = aichatmodule.Agent
	Provider                 = aichatmodule.Provider
	GormRepository           = aichatmodule.GormRepository
	Dependencies             = aichatmodule.Dependencies
	Service                  = aichatmodule.Service
)
