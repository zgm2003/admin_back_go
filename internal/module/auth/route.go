package auth

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service SessionService) {
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/auth")
	v1.GET("/login-config", handler.LoginConfig)
	v1.POST("/login", handler.Login)
	v1.POST("/refresh", handler.Refresh)
	v1.POST("/logout", handler.Logout)

	legacy := router.Group("/api/Users")
	legacy.POST("/getLoginConfig", handler.LoginConfig)
	legacy.POST("/login", handler.Login)
	legacy.POST("/refresh", handler.Refresh)
	legacy.POST("/logout", handler.Logout)
}
