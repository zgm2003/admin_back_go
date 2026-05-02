package auth

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service SessionService) {
	handler := NewHandler(service)

	v1 := router.Group("/api/v1/auth")
	v1.POST("/refresh", handler.Refresh)
	v1.POST("/logout", handler.Logout)

	legacy := router.Group("/api/Users")
	legacy.POST("/refresh", handler.Refresh)
	legacy.POST("/logout", handler.Logout)
}
