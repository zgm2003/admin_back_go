package admin

import crontaskmodule "admin_back_go/internal/module/crontask"

type (
	ListQuery         = crontaskmodule.ListQuery
	LogsQuery         = crontaskmodule.LogsQuery
	SaveInput         = crontaskmodule.SaveInput
	Page              = crontaskmodule.Page
	InitResponse      = crontaskmodule.InitResponse
	InitDict          = crontaskmodule.InitDict
	ListResponse      = crontaskmodule.ListResponse
	ListItem          = crontaskmodule.ListItem
	LogsResponse      = crontaskmodule.LogsResponse
	LogItem           = crontaskmodule.LogItem
	HTTPService       = crontaskmodule.HTTPService
	Task              = crontaskmodule.Task
	TaskLog           = crontaskmodule.TaskLog
	RegistryEntry     = crontaskmodule.RegistryEntry
	Registry          = crontaskmodule.Registry
	Repository        = crontaskmodule.Repository
	GormRepository    = crontaskmodule.GormRepository
	ScheduleRegistrar = crontaskmodule.ScheduleRegistrar
	SchedulerService  = crontaskmodule.SchedulerService
	Service           = crontaskmodule.Service
)

const (
	CommonYes        = crontaskmodule.CommonYes
	CommonNo         = crontaskmodule.CommonNo
	LogStatusSuccess = crontaskmodule.LogStatusSuccess
	LogStatusFailed  = crontaskmodule.LogStatusFailed
	LogStatusRunning = crontaskmodule.LogStatusRunning
)
