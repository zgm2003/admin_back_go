package usersession

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/user-sessions")
	group.GET("/page-init", handler.PageInit)
	group.GET("/stats", handler.Stats)
	group.GET("", handler.List)
}
