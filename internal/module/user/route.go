package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service InitService) {
	handler := NewHandler(service)

	legacy := router.Group("/api/Users")
	legacy.POST("/init", handler.Init)

	users := router.Group("/api/admin/v1/users")
	users.GET("/init", handler.Init)
	users.GET("/me", handler.Me)
}
