package server

import (
	mailadmin "admin_back_go/internal/module/mail/transport/admin"
	notificationadmin "admin_back_go/internal/module/notification/transport/admin"
	smsadmin "admin_back_go/internal/module/sms/transport/admin"
	uploadconfigadmin "admin_back_go/internal/module/uploadconfig/transport/admin"
	uploadtokenadmin "admin_back_go/internal/module/uploadtoken/transport/admin"
	uploadtokenapp "admin_back_go/internal/module/uploadtoken/transport/app"

	"github.com/gin-gonic/gin"
)

func registerAdminCommsRoutes(router *gin.Engine, deps Dependencies) {
	communications := deps.Admin.Communications
	notificationadmin.RegisterRoutes(router, communications.Notifications)
	notificationadmin.RegisterTaskRoutes(router, communications.NotificationTasks)
	mailadmin.RegisterRoutes(router, communications.Mail)
	smsadmin.RegisterRoutes(router, communications.SMS)
	uploadconfigadmin.RegisterRoutes(router, communications.UploadConfig)
	uploadtokenadmin.RegisterRoutes(router, communications.UploadTokens)
	uploadtokenapp.RegisterRoutes(router, communications.UploadTokens)
}
