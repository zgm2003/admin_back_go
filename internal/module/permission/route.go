package permission

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service ManagementService) {
	handler := NewManagementHandler(service)

	v1 := router.Group("/api/v1/permissions")
	v1.GET("/init", handler.Init)
	v1.GET("", handler.List)
	v1.POST("", handler.Create)
	v1.PUT("/:id", handler.Update)
	v1.PATCH("/:id/status", handler.ChangeStatus)
	v1.DELETE("/:id", handler.DeleteOne)
	v1.DELETE("", handler.DeleteBatch)
}
