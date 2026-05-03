package captcha

import "github.com/gin-gonic/gin"

// RegisterRoutes registers public CAPTCHA endpoints.
func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/auth")
	v1.GET("/captcha", handler.Generate)
}
