package admin

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/wallet")
	group.GET("/summary", handler.Summary)
	group.GET("/transactions", handler.Transactions)
	group.POST("/consumptions", handler.Consume)
	group.GET("/users/page-init", handler.WalletUsersPageInit)
	group.GET("/users", handler.WalletUsers)
	group.GET("/ledger/page-init", handler.LedgerPageInit)
	group.GET("/ledger", handler.Ledger)
}
