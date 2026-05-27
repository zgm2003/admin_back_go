package server

import (
	"admin_back_go/internal/module/clientversion"
	"admin_back_go/internal/module/crontask"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/operationlog"
	"admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/module/realtime"
	"admin_back_go/internal/module/system"
	"admin_back_go/internal/module/systemlog"
	"admin_back_go/internal/module/systemsetting"

	"github.com/gin-gonic/gin"
)

func registerAdminFoundationRoutes(router *gin.Engine, deps Dependencies) {
	system.RegisterRoutes(router, deps.Readiness)
	clientversion.RegisterRoutes(router, deps.ClientVersionService)
	exporttask.RegisterRoutes(router, deps.ExportTaskService)
	crontask.RegisterRoutes(router, deps.CronTaskService)
	operationlog.RegisterRoutes(router, deps.OperationLogService)
	queuemonitor.RegisterRoutes(router, deps.QueueMonitorService, deps.QueueMonitorUI)
	systemsetting.RegisterRoutes(router, deps.SystemSettingService)
	systemlog.RegisterRoutes(router, deps.SystemLogService)
	realtime.RegisterRoutes(router, deps.RealtimeHandler)
}
