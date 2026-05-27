package server

import (
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/payment"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/role"
	"admin_back_go/internal/module/wallet"

	"github.com/gin-gonic/gin"
)

func registerAdminCommerceRBACRoutes(router *gin.Engine, deps Dependencies) {
	payment.RegisterRoutes(router, deps.PaymentService)
	wallet.RegisterRoutes(router, deps.WalletService)
	permission.RegisterRoutes(router, deps.PermissionService)
	role.RegisterRoutes(router, deps.RoleService)
	authplatform.RegisterRoutes(router, deps.AuthPlatformService)
}
