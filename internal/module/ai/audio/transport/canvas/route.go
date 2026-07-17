package canvas

import (
	"net/http"

	aiaudiomodule "admin_back_go/internal/module/ai/audio"
	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aiaudiomodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	if router == nil {
		return
	}
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/audios",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.AudioGenerations)
}
