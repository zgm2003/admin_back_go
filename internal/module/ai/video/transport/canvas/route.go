package canvas

import (
	"net/http"

	aivideomodule "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aivideomodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/videos",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.VideoGenerations)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/videos/reference-media",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.ReferenceMediaUpload)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/ai/videos/:id",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.VideoStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/canvas/v1/ai/videos/:id/content",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.VideoContent)
}
