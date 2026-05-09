package aiagent

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/ai-agents")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.GET("/options", handler.Options)
	group.GET("/provider-models/:id", handler.ProviderModels)
	group.GET("/:id", handler.Detail)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.POST("/:id/test", handler.Test)
	group.DELETE("/:id", handler.Delete)
	group.GET("/:id/bindings", handler.Bindings)
	group.POST("/:id/bindings", handler.CreateBinding)

	router.DELETE("/api/admin/v1/ai-agent-bindings/:id", handler.DeleteBinding)
}
