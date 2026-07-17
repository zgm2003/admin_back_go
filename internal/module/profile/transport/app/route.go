package app

import (
	"net/http"

	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.AppService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/app/v1/profile",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
	}, handler.Profile)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/app/v1/profile",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("temporary retired App route"),
	}, handler.UpdateProfile)
}
