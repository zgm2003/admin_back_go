package server

import (
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/notification"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/sms"
	"admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/module/uploadtoken"

	"github.com/gin-gonic/gin"
)

func registerAdminCommsRoutes(router *gin.Engine, deps Dependencies) {
	notification.RegisterRoutes(router, deps.NotificationService)
	notificationtask.RegisterRoutes(router, deps.NotificationTaskService)
	mail.RegisterRoutes(router, deps.MailService)
	sms.RegisterRoutes(router, deps.SmsService)
	uploadconfig.RegisterRoutes(router, deps.UploadConfigService)
	uploadtoken.RegisterRoutes(router, deps.UploadTokenService)
}
