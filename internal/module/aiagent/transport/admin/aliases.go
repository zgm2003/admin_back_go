package admin

import aiagentmodule "admin_back_go/internal/module/aiagent"

type (
	InitResponse           = aiagentmodule.InitResponse
	InitDict               = aiagentmodule.InitDict
	EngineOption           = aiagentmodule.EngineOption
	ModelOption            = aiagentmodule.ModelOption
	ListQuery              = aiagentmodule.ListQuery
	Page                   = aiagentmodule.Page
	ListResponse           = aiagentmodule.ListResponse
	DetailResponse         = aiagentmodule.DetailResponse
	AgentDTO               = aiagentmodule.AgentDTO
	CreateInput            = aiagentmodule.CreateInput
	UpdateInput            = aiagentmodule.UpdateInput
	OptionQuery            = aiagentmodule.OptionQuery
	AgentOption            = aiagentmodule.AgentOption
	AgentOptionsResponse   = aiagentmodule.AgentOptionsResponse
	ConnectionTester       = aiagentmodule.ConnectionTester
	HTTPService            = aiagentmodule.HTTPService
	ProviderModelsResponse = aiagentmodule.ProviderModelsResponse
	ProviderModelDTO       = aiagentmodule.ProviderModelDTO
	Agent                  = aiagentmodule.Agent
	Provider               = aiagentmodule.Provider
	ProviderModel          = aiagentmodule.ProviderModel
	AgentWithProvider      = aiagentmodule.AgentWithProvider
	Repository             = aiagentmodule.Repository
	GormRepository         = aiagentmodule.GormRepository
	Service                = aiagentmodule.Service
)
