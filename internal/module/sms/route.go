package sms

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/sms")
	group.GET("/page-init", handler.PageInit)
	group.GET("/config", handler.Config)
	group.PUT("/config", handler.SaveConfig)
	group.DELETE("/config", handler.DeleteConfig)
	group.POST("/test", handler.TestSend)
	group.GET("/templates", handler.Templates)
	group.POST("/templates", handler.CreateTemplate)
	group.PUT("/templates/:id", handler.UpdateTemplate)
	group.PATCH("/templates/:id/status", handler.ChangeTemplateStatus)
	group.DELETE("/templates/:id", handler.DeleteTemplate)
	group.GET("/logs", handler.Logs)
	group.GET("/logs/:id", handler.Log)
	group.DELETE("/logs/:id", handler.DeleteLog)
	group.DELETE("/logs", handler.DeleteLogs)
}
