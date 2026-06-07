package admin

import (
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiimagemodule.HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/ai-images")
	group.GET("/page-init", handler.PageInit)
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.POST("", handler.Create)
	group.DELETE("/:id", handler.Delete)
}
