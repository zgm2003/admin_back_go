package admin

import (
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service auth.SessionService, captchaService auth.CaptchaHTTPService) {
	validate.MustRegister()
	handler := NewHandler(service, captchaService)

	v1 := router.Group("/api/admin/v1/auth")
	v1.GET("/login-config", handler.LoginConfig)
	v1.GET("/captcha", handler.Captcha)
	v1.POST("/send-code", handler.SendCode)
	v1.POST("/forgot-password", handler.ForgetPassword)
	v1.POST("/login", handler.Login)
	v1.POST("/refresh", handler.Refresh)
	v1.POST("/logout", handler.Logout)
}
