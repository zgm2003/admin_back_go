package systemlog

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/system-logs")
	v1.GET("/init", handler.Init)
	v1.GET("/files", handler.Files)
	v1.GET("/files/:name/lines", handler.Lines)
}
