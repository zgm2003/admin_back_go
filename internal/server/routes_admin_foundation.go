package server

import (
	clientversionadmin "admin_back_go/internal/module/clientversion/transport/admin"
	crontaskadmin "admin_back_go/internal/module/crontask/transport/admin"
	exporttaskadmin "admin_back_go/internal/module/exporttask/transport/admin"
	operationlogadmin "admin_back_go/internal/module/operationlog/transport/admin"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	realtimeadmin "admin_back_go/internal/module/realtime/transport/admin"
	systemadmin "admin_back_go/internal/module/system/transport/admin"
	systemlogadmin "admin_back_go/internal/module/systemlog/transport/admin"
	systemsettingadmin "admin_back_go/internal/module/systemsetting/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminFoundationRoutes(router *gin.Engine, deps Dependencies) {
	systemadmin.RegisterRoutes(router, deps.Readiness)
	clientversionadmin.RegisterRoutes(router, deps.ClientVersionService)
	exporttaskadmin.RegisterRoutes(router, deps.ExportTaskService)
	crontaskadmin.RegisterRoutes(router, deps.CronTaskService)
	operationlogadmin.RegisterRoutes(router, deps.OperationLogService)
	queuemonitoradmin.RegisterRoutes(router, deps.QueueMonitorService, deps.QueueMonitorUI)
	systemsettingadmin.RegisterRoutes(router, deps.SystemSettingService)
	systemlogadmin.RegisterRoutes(router, deps.SystemLogService)
	realtimeadmin.RegisterRoutes(router, deps.RealtimeHandler)
}
