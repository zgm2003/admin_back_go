package uploadconfig

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	drivers := router.Group("/api/admin/v1/upload-drivers")
	drivers.GET("/init", handler.DriverInit)
	drivers.GET("", handler.DriverList)
	drivers.POST("", handler.DriverCreate)
	drivers.PUT("/:id", handler.DriverUpdate)
	drivers.DELETE("/:id", handler.DriverDeleteOne)
	drivers.DELETE("", handler.DriverDeleteBatch)

	rules := router.Group("/api/admin/v1/upload-rules")
	rules.GET("/init", handler.RuleInit)
	rules.GET("", handler.RuleList)
	rules.POST("", handler.RuleCreate)
	rules.PUT("/:id", handler.RuleUpdate)
	rules.DELETE("/:id", handler.RuleDeleteOne)
	rules.DELETE("", handler.RuleDeleteBatch)

	settings := router.Group("/api/admin/v1/upload-settings")
	settings.GET("/init", handler.SettingInit)
	settings.GET("", handler.SettingList)
	settings.POST("", handler.SettingCreate)
	settings.PUT("/:id", handler.SettingUpdate)
	settings.PATCH("/:id/status", handler.SettingChangeStatus)
	settings.DELETE("/:id", handler.SettingDeleteOne)
	settings.DELETE("", handler.SettingDeleteBatch)
}
