package canvas

import (
	"net/http"

	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aiimagemodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/ai/images",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/images/generations",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Generations)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/images/edits",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Edits)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/ai/images/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Status)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/canvas/v1/ai/images/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.Delete)
}
