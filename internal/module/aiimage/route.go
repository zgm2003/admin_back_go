package aiimage

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-images")
	group.GET("/page-init", handler.PageInit)
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.POST("/assets", handler.RegisterAsset)
	group.POST("", handler.Create)
	group.PATCH("/:id/favorite", handler.Favorite)
	group.DELETE("/:id", handler.Delete)
}
