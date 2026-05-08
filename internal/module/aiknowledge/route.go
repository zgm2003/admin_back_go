package aiknowledge

import (
	"admin_back_go/internal/validate"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	h := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-knowledge-bases")
	group.GET("/page-init", h.Init)
	group.GET("", h.List)
	group.POST("", h.Create)
	group.GET("/:id", h.Detail)
	group.PUT("/:id", h.Update)
	group.PATCH("/:id/status", h.ChangeStatus)
	group.DELETE("/:id", h.Delete)
	group.GET("/:id/documents", h.Documents)
	group.POST("/:id/documents", h.CreateDocument)
	group.GET("/:id/documents/:document_id", h.DocumentDetail)
	group.PUT("/:id/documents/:document_id", h.UpdateDocument)
	group.DELETE("/:id/documents/:document_id", h.DeleteDocument)
	group.POST("/:id/documents/:document_id/reindex", h.ReindexDocument)
	group.GET("/:id/chunks", h.Chunks)
	group.POST("/:id/retrieval-test", h.RetrievalTest)
}
