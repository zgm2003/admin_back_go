package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service InitService) {
	handler := NewHandler(service)

	legacy := router.Group("/api/Users")
	legacy.POST("/init", handler.Init)
}
