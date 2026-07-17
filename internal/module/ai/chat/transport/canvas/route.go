package canvas

import (
	"net/http"

	aichatmodule "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service aichatmodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/canvas/v1/ai/chat/completions",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired Canvas route"),
	}, handler.ChatCompletions)
}
