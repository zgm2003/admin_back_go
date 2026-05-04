package auth

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service SessionService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/auth")
	v1.GET("/login-config", handler.LoginConfig)
	v1.POST("/send-code", handler.SendCode)
	v1.POST("/login", handler.Login)
	v1.POST("/refresh", handler.Refresh)
	v1.POST("/logout", handler.Logout)

	legacy := router.Group("/api/Users")
	legacy.POST("/getLoginConfig", handler.LoginConfig)
	legacy.POST("/sendCode", handler.SendCode)
	legacy.POST("/login", handler.Login)
	legacy.POST("/refresh", handler.Refresh)
	legacy.POST("/logout", handler.Logout)
}
