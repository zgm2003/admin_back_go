package queuemonitor

import (
	"net/http"

	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, monitor http.Handler) {
	validate.MustRegister()
	handler := NewHandler(service, monitor)

	v1 := router.Group("/api/admin/v1/queue-monitor")
	v1.GET("", handler.List)
	v1.GET("/failed", handler.FailedList)
	router.Any(UIPath, handler.UI)
	router.Any(UIPath+"/*path", handler.UI)
}
