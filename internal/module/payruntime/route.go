package payruntime

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/recharge-orders")
	group.GET("", handler.ListCurrentUserRechargeOrders)
	group.POST("", handler.CreateRechargeOrder)
	group.GET("/:order_no/result", handler.QueryCurrentUserRechargeResult)
	group.PATCH("/:order_no/cancel", handler.CancelCurrentUserRechargeOrder)
	group.POST("/:order_no/pay-attempts", handler.CreatePayAttempt)

	router.GET("/api/admin/v1/wallet/summary", handler.CurrentUserWalletSummary)
	router.GET("/api/admin/v1/wallet/bills", handler.CurrentUserWalletBills)
	router.POST("/api/pay/notify/alipay", handler.AlipayNotify)
}
