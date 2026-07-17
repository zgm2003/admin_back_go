package callback

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/payment/callbacks/alipay",
		Access: adminroute.Public(),
		Audit:  adminroute.NoAudit("public provider callback; domain verification retained"),
	}, handler.AlipayCallback)
}
