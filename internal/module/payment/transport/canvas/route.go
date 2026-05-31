package canvas

import (
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRechargeRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	recharges := router.Group("/api/canvas/v1/payment/recharges")
	recharges.GET("/page-init", handler.RechargePageInit)
	recharges.GET("", handler.ListRecharges)
	recharges.POST("", handler.CreateRecharge)
	recharges.POST("/:id/pay", handler.PayRecharge)
}
