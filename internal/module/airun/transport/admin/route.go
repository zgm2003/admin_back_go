package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-runs")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.GET("/stats", handler.Stats)
	group.GET("/stats/by-date", handler.StatsByDate)
	group.GET("/stats/by-agent", handler.StatsByAgent)
	group.GET("/stats/by-user", handler.StatsByUser)
	group.GET("/:id", handler.Detail)
}
