package admin

import systemsettingmodule "admin_back_go/internal/module/systemsetting"

type (
	InitResponse   = systemsettingmodule.InitResponse
	InitDict       = systemsettingmodule.InitDict
	ListQuery      = systemsettingmodule.ListQuery
	Page           = systemsettingmodule.Page
	ListResponse   = systemsettingmodule.ListResponse
	ListItem       = systemsettingmodule.ListItem
	CreateInput    = systemsettingmodule.CreateInput
	UpdateInput    = systemsettingmodule.UpdateInput
	Setting        = systemsettingmodule.Setting
	Repository     = systemsettingmodule.Repository
	GormRepository = systemsettingmodule.GormRepository
	Service        = systemsettingmodule.Service
)
