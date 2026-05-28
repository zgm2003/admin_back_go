package callback

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := NewHandler(service)
	callbacks := router.Group("/api/payment/callbacks")
	callbacks.POST("/alipay", handler.AlipayCallback)
}
