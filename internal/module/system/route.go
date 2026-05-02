package system

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, readiness ReadinessChecker) {
	service := NewService(readiness)
	handler := NewHandler(service)

	router.GET("/health", handler.Health)
	router.GET("/ready", handler.Ready)

	api := router.Group("/api/v1")
	api.GET("/ping", handler.Ping)
}
