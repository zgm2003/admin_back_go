package appauth

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, uploadTokenService UploadTokenService) {
	validate.MustRegister()
	handler := NewHandler(uploadTokenService)

	uploadTokens := router.Group("/api/app/v1/upload-tokens")
	uploadTokens.POST("", handler.CreateUploadToken)
}
