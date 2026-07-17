package admin

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	registerCurrentUserRoutes(routes, handler)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/ledger/page-init",
		Access: adminroute.Permission("payment_ledger_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.LedgerPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/ledger",
		Access: adminroute.Permission("payment_ledger_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Ledger)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/wallets/page-init",
		Access: adminroute.Permission("payment_wallet_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.WalletUsersPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/wallets",
		Access: adminroute.Permission("payment_wallet_list"),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.WalletUsers)
}

func registerCurrentUserRoutes(routes adminroute.Registrar, handler *Handler) {
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/wallet/summary",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Summary)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/wallet/transactions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Transactions)
}
