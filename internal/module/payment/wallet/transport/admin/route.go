package admin

import (
	"net/http"

	walletmodule "admin_back_go/internal/module/payment/wallet"
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
		Contract: &adminroute.HTTPContract{
			Response: walletmodule.LedgerPageInitResponse{},
		},
	}, handler.LedgerPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/ledger",
		Access: adminroute.Permission("payment_ledger_list"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    transactionListRequest{},
			Response: walletmodule.TransactionListResponse{},
		},
	}, handler.Ledger)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/wallets/page-init",
		Access: adminroute.Permission("payment_wallet_list"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: walletmodule.WalletUsersPageInitResponse{},
		},
	}, handler.WalletUsersPageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/payment/wallets",
		Access: adminroute.Permission("payment_wallet_list"),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    walletUserListRequest{},
			Response: walletmodule.WalletUserListResponse{},
		},
	}, handler.WalletUsers)
}

func registerCurrentUserRoutes(routes adminroute.Registrar, handler *Handler) {
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/wallet/summary",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: walletmodule.SummaryResponse{},
		},
	}, handler.Summary)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/wallet/transactions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    transactionListRequest{},
			Response: walletmodule.TransactionListResponse{},
		},
	}, handler.Transactions)
}
