package server

import (
	clientversionadmin "admin_back_go/internal/module/clientversion/transport/admin"
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
	systemadmin.RegisterRoutes(router, deps.Core.Readiness)
	clientversionadmin.RegisterRoutes(router, system.ClientVersions)
	exporttaskadmin.RegisterRoutes(router, system.Exports)
	crontaskadmin.RegisterRoutes(router, system.CronTasks)
	operationlogadmin.RegisterRoutes(router, system.OperationLogs)
	queuemonitoradmin.RegisterRoutes(router, system.QueueMonitor, deps.Core.QueueMonitorUI)
	systemsettingadmin.RegisterRoutes(router, system.Settings)
	systemlogadmin.RegisterRoutes(router, system.Logs)
	realtimeadmin.RegisterRoutes(router, deps.Core.RealtimeHandler)
}
