package server

import (
	authplatformadmin "admin_back_go/internal/module/authplatform/transport/admin"
	paymentadmin "admin_back_go/internal/module/payment/transport/admin"
	paymentcallback "admin_back_go/internal/module/payment/transport/callback"
	permissionadmin "admin_back_go/internal/module/permission/transport/admin"
	roleadmin "admin_back_go/internal/module/role/transport/admin"
	walletadmin "admin_back_go/internal/module/wallet/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminCommerceRBACRoutes(router *gin.Engine, deps Dependencies) {
	paymentcallback.RegisterRoutes(router, deps.PaymentService)
	paymentadmin.RegisterRoutes(router, deps.PaymentService)
	walletadmin.RegisterRoutes(router, deps.WalletService)
	permissionadmin.RegisterRoutes(router, deps.PermissionService)
	roleadmin.RegisterRoutes(router, deps.RoleService)
	authplatformadmin.RegisterRoutes(router, deps.AuthPlatformService)
}
