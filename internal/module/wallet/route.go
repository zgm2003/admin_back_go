package wallet

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	wallets := router.Group("/api/admin/v1/wallets")
	wallets.GET("/page-init", handler.Init)
	wallets.GET("", handler.List)

	router.GET("/api/admin/v1/wallet-transactions", handler.Transactions)
	router.POST("/api/admin/v1/wallet-adjustments", handler.CreateAdjustment)
}
