package admin

import clientversionmodule "admin_back_go/internal/module/clientversion"

type (
	Page                       = clientversionmodule.Page
	InitResponse               = clientversionmodule.InitResponse
	InitDict                   = clientversionmodule.InitDict
	ListQuery                  = clientversionmodule.ListQuery
	ListResponse               = clientversionmodule.ListResponse
	ListItem                   = clientversionmodule.ListItem
	CreateInput                = clientversionmodule.CreateInput
	UpdateInput                = clientversionmodule.UpdateInput
	CurrentCheckQuery          = clientversionmodule.CurrentCheckQuery
	CurrentCheckResponse       = clientversionmodule.CurrentCheckResponse
	ManifestPlatform           = clientversionmodule.ManifestPlatform
	ManifestPayload            = clientversionmodule.ManifestPayload
	HTTPService                = clientversionmodule.HTTPService
	Version                    = clientversionmodule.Version
	ManifestCOSPublisher       = clientversionmodule.ManifestCOSPublisher
	Repository                 = clientversionmodule.Repository
	GormRepository             = clientversionmodule.GormRepository
	ManifestPublisher          = clientversionmodule.ManifestPublisher
	Service                    = clientversionmodule.Service
	UploadConfig               = clientversionmodule.UploadConfig
	UploadConfigRepository     = clientversionmodule.UploadConfigRepository
	GormUploadConfigRepository = clientversionmodule.GormUploadConfigRepository
)
