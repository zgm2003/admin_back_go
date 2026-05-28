package admin

import aitoolmodule "admin_back_go/internal/module/aitool"

type (
	JSONObject               = aitoolmodule.JSONObject
	InitResponse             = aitoolmodule.InitResponse
	InitDict                 = aitoolmodule.InitDict
	ListQuery                = aitoolmodule.ListQuery
	Page                     = aitoolmodule.Page
	ListResponse             = aitoolmodule.ListResponse
	ToolDTO                  = aitoolmodule.ToolDTO
	MutationInput            = aitoolmodule.MutationInput
	GeneratePageInitResponse = aitoolmodule.GeneratePageInitResponse
	GenerateAgentOption      = aitoolmodule.GenerateAgentOption
	GenerateDraftInput       = aitoolmodule.GenerateDraftInput
	GenerateDraftResponse    = aitoolmodule.GenerateDraftResponse
	GeneratedToolDraft       = aitoolmodule.GeneratedToolDraft
	GenerateUsage            = aitoolmodule.GenerateUsage
	GenerateAgentConfig      = aitoolmodule.GenerateAgentConfig
	EngineConfig             = aitoolmodule.EngineConfig
	EngineFactory            = aitoolmodule.EngineFactory
	AgentToolsResponse       = aitoolmodule.AgentToolsResponse
	UpdateAgentToolsInput    = aitoolmodule.UpdateAgentToolsInput
	RuntimeToolRow           = aitoolmodule.RuntimeToolRow
	RuntimeTool              = aitoolmodule.RuntimeTool
	StartToolCallInput       = aitoolmodule.StartToolCallInput
	FinishToolCallInput      = aitoolmodule.FinishToolCallInput
	ExecuteInput             = aitoolmodule.ExecuteInput
	ExecuteResult            = aitoolmodule.ExecuteResult
	UserCount                = aitoolmodule.UserCount
	Executor                 = aitoolmodule.Executor
	HTTPService              = aitoolmodule.HTTPService
	RuntimeService           = aitoolmodule.RuntimeService
	AdminUserCountExecutor   = aitoolmodule.AdminUserCountExecutor
	Tool                     = aitoolmodule.Tool
	AgentTool                = aitoolmodule.AgentTool
	ToolCall                 = aitoolmodule.ToolCall
	Agent                    = aitoolmodule.Agent
	Repository               = aitoolmodule.Repository
	GormRepository           = aitoolmodule.GormRepository
	Service                  = aitoolmodule.Service
	Option                   = aitoolmodule.Option
)
