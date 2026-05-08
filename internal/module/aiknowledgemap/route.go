package aiknowledgemap

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/ai-knowledge-maps")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.GET("/:id", handler.Detail)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.POST("/:id/sync", handler.Sync)
	group.DELETE("/:id", handler.Delete)
	group.GET("/:id/documents", handler.Documents)
	group.POST("/:id/documents", handler.CreateDocument)

	docGroup := router.Group("/api/admin/v1/ai-knowledge-documents")
	docGroup.PATCH("/:id/status", handler.ChangeDocumentStatus)
	docGroup.POST("/:id/refresh-status", handler.RefreshDocumentStatus)
	docGroup.DELETE("/:id", handler.DeleteDocument)
}
