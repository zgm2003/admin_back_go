package admin

import (
	systemmodule "admin_back_go/internal/module/system"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, readiness systemmodule.ReadinessChecker) {
	service := systemmodule.NewService(readiness)
	handler := NewHandler(service)

	router.GET("/health", handler.Health)
	router.GET("/ready", handler.Ready)

	api := router.Group("/api/admin/v1")
	api.GET("/ping", handler.Ping)
}
