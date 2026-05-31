package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	registerCurrentUserRoutes(router, "/api/admin/v1/wallet", handler)

	ledger := router.Group("/api/admin/v1/payment/ledger")
	ledger.GET("/page-init", handler.LedgerPageInit)
	ledger.GET("", handler.Ledger)

	wallets := router.Group("/api/admin/v1/payment/wallets")
	wallets.GET("/page-init", handler.WalletUsersPageInit)
	wallets.GET("", handler.WalletUsers)
}

func RegisterCurrentUserRoutes(router *gin.Engine, prefix string, service HTTPService) {
	validate.MustRegister()
	registerCurrentUserRoutes(router, prefix, NewHandler(service))
}

func registerCurrentUserRoutes(router *gin.Engine, prefix string, handler *Handler) {
	current := router.Group(prefix)
	current.GET("/summary", handler.Summary)
	current.GET("/transactions", handler.Transactions)
}
