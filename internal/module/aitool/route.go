package aitool

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/ai-tools")
	group.GET("/page-init", handler.Init)
	group.GET("/generate/page-init", handler.GeneratePageInit)
	group.GET("", handler.List)
	group.POST("/generate-draft", handler.GenerateDraft)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/status", handler.ChangeStatus)
	group.DELETE("/:id", handler.Delete)

	agentGroup := router.Group("/api/admin/v1/ai-agents")
	agentGroup.GET("/:id/tools", handler.AgentTools)
	agentGroup.PUT("/:id/tools", handler.UpdateAgentTools)
}
