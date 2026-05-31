package canvas

import (
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Service profile.AppService
}

func RegisterRoutes(router *gin.Engine, deps Dependencies) {
	validate.MustRegister()
	handler := NewHandler(deps.Service)
	users := router.Group("/api/canvas/v1/users")
	users.GET("/me", handler.Me)
}
