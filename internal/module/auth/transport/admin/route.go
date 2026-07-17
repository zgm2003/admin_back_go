package admin

import (
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service auth.SessionService, captchaService auth.CaptchaHTTPService, sessionAdminService auth.SessionAdminHTTPService, loginLogService auth.LoginLogHTTPService, options ...Option) {
	validate.MustRegister()
	handler := NewHandler(service, captchaService, sessionAdminService, loginLogService, options...)

	v1 := router.Group("/api/admin/v1/auth")
	v1.GET("/login-config", handler.LoginConfig)
	v1.GET("/captcha", handler.Captcha)
	v1.POST("/send-code", handler.SendCode)
	v1.POST("/forgot-password", handler.ForgetPassword)
	v1.POST("/login", handler.Login)
	v1.POST("/refresh", handler.Refresh)
	v1.POST("/logout", handler.Logout)
	v1.POST("/realtime-tickets", handler.RealtimeTicket)
	v1.POST("/queue-monitor-grants", handler.QueueMonitorGrant)

	sessions := router.Group("/api/admin/v1/user-sessions")
	sessions.GET("/page-init", handler.SessionPageInit)
	sessions.GET("/stats", handler.SessionStats)
	sessions.PATCH("/revoke", handler.SessionBatchRevoke)
	sessions.GET("", handler.SessionList)
	sessions.PATCH("/:id/revoke", handler.SessionRevoke)

	loginLogs := router.Group("/api/admin/v1/users/login-logs")
	loginLogs.GET("/page-init", handler.LoginLogPageInit)
	loginLogs.GET("", handler.LoginLogList)
}
