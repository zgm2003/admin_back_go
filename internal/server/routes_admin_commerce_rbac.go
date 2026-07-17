package server

import (
	authplatformadmin "admin_back_go/internal/module/auth_platform/transport/admin"
	paymentadmin "admin_back_go/internal/module/payment/transport/admin"
	paymentcallback "admin_back_go/internal/module/payment/transport/callback"
	paymentcanvas "admin_back_go/internal/module/payment/transport/canvas"
	walletadmin "admin_back_go/internal/module/payment/wallet/transport/admin"
	walletcanvas "admin_back_go/internal/module/payment/wallet/transport/canvas"
	permissionadmin "admin_back_go/internal/module/permission/transport/admin"
	roleadmin "admin_back_go/internal/module/role/transport/admin"

	"github.com/gin-gonic/gin"
)

func registerAdminCommerceRBACRoutes(router *gin.Engine, deps Dependencies) {
	commerce := deps.Admin.Commerce
	identity := deps.Admin.Identity
	paymentcallback.RegisterRoutes(router, commerce.Payment, deps.Core.RouteRegistry)
	paymentadmin.RegisterRoutes(router, commerce.Payment, deps.Core.RouteRegistry)
	walletadmin.RegisterRoutes(router, commerce.Wallet, deps.Core.RouteRegistry)
	permissionadmin.RegisterRoutes(router, identity.Permissions, deps.Core.RouteRegistry)
	roleadmin.RegisterRoutes(router, identity.Roles, deps.Core.RouteRegistry)
	authplatformadmin.RegisterRoutes(router, identity.AuthPlatforms, deps.Core.RouteRegistry)
	walletcanvas.RegisterRoutes(router, commerce.Wallet, deps.Core.RouteRegistry)
	paymentcanvas.RegisterRechargeRoutes(router, commerce.Payment, deps.Core.RouteRegistry)
}
