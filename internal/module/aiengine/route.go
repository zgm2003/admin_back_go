package aiengine

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-engine-connections")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.POST("/model-options", handler.PreviewModels)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.POST("/:id/model-options", handler.PreviewStoredModels)
	group.POST("/:id/test", handler.TestConnection)
	group.POST("/:id/sync-models", handler.SyncModels)
	group.GET("/:id/models", handler.ListProviderModels)
	group.PUT("/:id/models", handler.UpdateProviderModels)
	group.DELETE("/:id", handler.Delete)
}
