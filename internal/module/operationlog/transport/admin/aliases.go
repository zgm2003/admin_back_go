package admin

import operationlogmodule "admin_back_go/internal/module/operationlog"

type (
	InitResponse   = operationlogmodule.InitResponse
	ListQuery      = operationlogmodule.ListQuery
	Page           = operationlogmodule.Page
	ListResponse   = operationlogmodule.ListResponse
	ListItem       = operationlogmodule.ListItem
	ListRow        = operationlogmodule.ListRow
	Log            = operationlogmodule.Log
	User           = operationlogmodule.User
	Repository     = operationlogmodule.Repository
	GormRepository = operationlogmodule.GormRepository
	Service        = operationlogmodule.Service
)
