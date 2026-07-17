package canvas

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
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/assets",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Assets)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/assets",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/canvas/v1/assets/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/canvas/v1/assets/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Delete)
}
