package canvas

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRechargeRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/payment/recharges/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.RechargePageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/payment/recharges",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.ListRecharges)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/payment/recharges",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.CreateRecharge)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/payment/recharges/:id/pay",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.PayRecharge)
}
