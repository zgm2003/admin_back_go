package admin

import aiimagemodule "admin_back_go/internal/module/ai/image"

type (
	Page                   = aiimagemodule.Page
	PageInitResponse       = aiimagemodule.PageInitResponse
	PageInitDict           = aiimagemodule.PageInitDict
	AgentOption            = aiimagemodule.AgentOption
	ListQuery              = aiimagemodule.ListQuery
	ListResponse           = aiimagemodule.ListResponse
	DetailResponse         = aiimagemodule.DetailResponse
	TaskDTO                = aiimagemodule.TaskDTO
	AssetDTO               = aiimagemodule.AssetDTO
	RegisterAssetInput     = aiimagemodule.RegisterAssetInput
	CreateInput            = aiimagemodule.CreateInput
	FavoriteInput          = aiimagemodule.FavoriteInput
	CreateTaskResponse     = aiimagemodule.CreateTaskResponse
	HTTPService            = aiimagemodule.HTTPService
	JobService             = aiimagemodule.JobService
	GeneratePayload        = aiimagemodule.GeneratePayload
	GenerateInput          = aiimagemodule.GenerateInput
	GenerateResult         = aiimagemodule.GenerateResult
	ImageTask              = aiimagemodule.ImageTask
	ImageAsset             = aiimagemodule.ImageAsset
	ImageTaskAsset         = aiimagemodule.ImageTaskAsset
	AgentRuntime           = aiimagemodule.AgentRuntime
	UploadConfig           = aiimagemodule.UploadConfig
	TaskAssetRow           = aiimagemodule.TaskAssetRow
	Repository             = aiimagemodule.Repository
	GormRepository         = aiimagemodule.GormRepository
	Service                = aiimagemodule.Service
	Dependencies           = aiimagemodule.Dependencies
	ImageEngineConfig      = aiimagemodule.ImageEngineConfig
	ImageEngineFactory     = aiimagemodule.ImageEngineFactory
	ImageEngineFactoryFunc = aiimagemodule.ImageEngineFactoryFunc
)
