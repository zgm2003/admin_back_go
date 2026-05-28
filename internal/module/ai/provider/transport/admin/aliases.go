package admin

import aiprovidermodule "admin_back_go/internal/module/ai/provider"

type (
	InitResponse           = aiprovidermodule.InitResponse
	InitDict               = aiprovidermodule.InitDict
	ListQuery              = aiprovidermodule.ListQuery
	Page                   = aiprovidermodule.Page
	ListResponse           = aiprovidermodule.ListResponse
	ProviderDTO            = aiprovidermodule.ProviderDTO
	ProviderModelDTO       = aiprovidermodule.ProviderModelDTO
	ModelOptionDTO         = aiprovidermodule.ModelOptionDTO
	ModelOptionsResponse   = aiprovidermodule.ModelOptionsResponse
	ProviderModelsResponse = aiprovidermodule.ProviderModelsResponse
	CreateInput            = aiprovidermodule.CreateInput
	UpdateInput            = aiprovidermodule.UpdateInput
	ModelOptionsInput      = aiprovidermodule.ModelOptionsInput
	UpdateModelsInput      = aiprovidermodule.UpdateModelsInput
	ProviderTester         = aiprovidermodule.ProviderTester
	ModelDriver            = aiprovidermodule.ModelDriver
	HTTPService            = aiprovidermodule.HTTPService
	Provider               = aiprovidermodule.Provider
	ProviderModel          = aiprovidermodule.ProviderModel
	Repository             = aiprovidermodule.Repository
	GormRepository         = aiprovidermodule.GormRepository
	Service                = aiprovidermodule.Service
)
