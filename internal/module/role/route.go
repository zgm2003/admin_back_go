package role

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/roles")
	v1.GET("/init", handler.Init)
	v1.GET("", handler.List)
	v1.POST("", handler.Create)
	v1.PUT("/:id", handler.Update)
	v1.PATCH("/:id/default", handler.SetDefault)
	v1.DELETE("/:id", handler.DeleteOne)
	v1.DELETE("", handler.DeleteBatch)
}

