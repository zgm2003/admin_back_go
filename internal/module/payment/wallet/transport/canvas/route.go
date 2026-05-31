package canvas

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	wallet := router.Group("/api/canvas/v1/wallet")
	wallet.GET("/summary", handler.Summary)
	wallet.GET("/transactions", handler.Transactions)
}
