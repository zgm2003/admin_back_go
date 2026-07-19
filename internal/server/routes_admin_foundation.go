package server

import (
	crontaskadmin "admin_back_go/internal/module/crontask/transport/admin"
	exporttaskadmin "admin_back_go/internal/module/export/transport/admin"
	operationlogadmin "admin_back_go/internal/module/operationlog/transport/admin"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	systemadmin "admin_back_go/internal/module/system/transport/admin"
	systemlogadmin "admin_back_go/internal/module/systemlog/transport/admin"
	systemsettingadmin "admin_back_go/internal/module/systemsetting/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminFoundationRoutes(router *gin.Engine, deps Dependencies) {
	system := deps.Admin.System
	systemadmin.RegisterRoutes(router, deps.Core.Readiness, deps.Core.RouteRegistry)
	exporttaskadmin.RegisterRoutes(router, system.Exports, deps.Core.RouteRegistry)
	crontaskadmin.RegisterRoutes(router, system.CronTasks, deps.Core.RouteRegistry)
	operationlogadmin.RegisterRoutes(router, system.OperationLogs, deps.Core.RouteRegistry)
	queuemonitoradmin.RegisterRoutes(router, system.QueueMonitor, deps.Core.QueueMonitorUI, deps.Core.RouteRegistry)
	systemsettingadmin.RegisterRoutes(router, system.Settings, deps.Core.RouteRegistry)
	systemlogadmin.RegisterRoutes(router, system.Logs, deps.Core.RouteRegistry)
	realtimeadmin.RegisterRoutes(router, deps.Core.RealtimeHandler, deps.Core.RouteRegistry)
}
