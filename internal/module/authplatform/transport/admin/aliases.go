package admin

import authplatformmodule "admin_back_go/internal/module/authplatform"

type (
	InitResponse         = authplatformmodule.InitResponse
	InitDict             = authplatformmodule.InitDict
	ListQuery            = authplatformmodule.ListQuery
	Page                 = authplatformmodule.Page
	ListResponse         = authplatformmodule.ListResponse
	ListItem             = authplatformmodule.ListItem
	CreateInput          = authplatformmodule.CreateInput
	UpdateInput          = authplatformmodule.UpdateInput
	Repository           = authplatformmodule.Repository
	ManagementRepository = authplatformmodule.ManagementRepository
	GormRepository       = authplatformmodule.GormRepository
	Platform             = authplatformmodule.Platform
	Service              = authplatformmodule.Service
)
