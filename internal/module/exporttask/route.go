package exporttask

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/export-tasks")
	v1.GET("/status-count", handler.StatusCount)
	v1.GET("", handler.List)
	v1.DELETE("/:id", handler.DeleteOne)
	v1.DELETE("", handler.DeleteBatch)
}
