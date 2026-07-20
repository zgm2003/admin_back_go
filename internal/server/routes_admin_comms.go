package server

import (
	mailadmin "admin_back_go/internal/module/mail/transport/admin"
	notificationadmin "admin_back_go/internal/module/notification/transport/admin"
	smsadmin "admin_back_go/internal/module/sms/transport/admin"
	uploadconfigadmin "admin_back_go/internal/module/uploadconfig/transport/admin"
	uploadtokenadmin "admin_back_go/internal/module/uploadtoken/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminCommsRoutes(router *gin.Engine, deps Dependencies) {
	communications := deps.Admin.Communications
	notificationadmin.RegisterRoutes(router, communications.Notifications, deps.Core.RouteRegistry)
	notificationadmin.RegisterTaskRoutes(router, communications.NotificationTasks, deps.Core.RouteRegistry)
	mailadmin.RegisterRoutes(router, communications.Mail, deps.Core.RouteRegistry)
	smsadmin.RegisterRoutes(router, communications.SMS, deps.Core.RouteRegistry)
	uploadconfigadmin.RegisterRoutes(router, communications.UploadConfig, deps.Core.RouteRegistry)
	uploadtokenadmin.RegisterRoutes(router, communications.UploadTokens, deps.Core.RouteRegistry)
}
