package clientversion

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	group := router.Group("/api/admin/v1/client-versions")
	group.GET("/page-init", handler.Init)
	group.GET("/update-json", handler.UpdateJSON)
	group.GET("/current-check", handler.CurrentCheck)
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.PUT("/:id", handler.Update)
	group.PATCH("/:id/latest", handler.SetLatest)
	group.PATCH("/:id/force-update", handler.ForceUpdate)
	group.DELETE("/:id", handler.Delete)
}
