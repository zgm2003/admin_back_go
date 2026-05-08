package payment

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	channels := router.Group("/api/admin/v1/payment/channels")
	channels.GET("/page-init", handler.ChannelInit)
	channels.GET("", handler.ListChannels)
	channels.POST("", handler.CreateChannel)
	channels.PUT("/:id", handler.UpdateChannel)
	channels.PATCH("/:id/status", handler.ChangeChannelStatus)
	channels.DELETE("/:id", handler.DeleteChannel)

	orders := router.Group("/api/admin/v1/payment/orders")
	orders.GET("/page-init", handler.OrderInit)
	orders.GET("", handler.ListOrders)
	orders.POST("", handler.CreateOrder)
	orders.GET("/:order_no/result", handler.GetOrderResult)
	orders.POST("/:order_no/pay", handler.PayOrder)
	orders.PATCH("/:order_no/cancel", handler.CancelOrder)
	orders.GET("/:order_no", handler.GetAdminOrder)
	orders.PATCH("/:order_no/close", handler.CloseAdminOrder)

	events := router.Group("/api/admin/v1/payment/events")
	events.GET("", handler.ListEvents)
	events.GET("/:id", handler.GetEvent)

	router.POST("/api/payment/notify/alipay", handler.AlipayNotify)
}
