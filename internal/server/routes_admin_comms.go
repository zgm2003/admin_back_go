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
	notificationadmin.RegisterRoutes(router, deps.NotificationService)
	notificationadmin.RegisterTaskRoutes(router, deps.NotificationTaskService)
	mailadmin.RegisterRoutes(router, deps.MailService)
	smsadmin.RegisterRoutes(router, deps.SmsService)
	uploadconfigadmin.RegisterRoutes(router, deps.UploadConfigService)
	uploadtokenadmin.RegisterRoutes(router, deps.UploadTokenService)
	uploadtokenapp.RegisterRoutes(router, deps.UploadTokenService)
}
