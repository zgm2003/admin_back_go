package admin

import (
	"net/http"

	uploadtokenmodule "admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, routeRegistries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)

	routes := adminroute.NewRegistrar(router, routeRegistries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/upload-tokens",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("self-service upload credential issuance"),
		Contract: &adminroute.HTTPContract{
			Request:  createRequest{},
			Response: uploadtokenmodule.CreateResponse{},
		},
	}, handler.Create)
}
