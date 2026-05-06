package payruntime

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/recharge-orders")
	group.POST("", handler.CreateRechargeOrder)
	group.POST("/:order_no/pay-attempts", handler.CreatePayAttempt)

	router.POST("/api/pay/notify/alipay", handler.AlipayNotify)
}
