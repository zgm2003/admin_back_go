package admin

import (
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service profile.HTTPService, quickEntryService profile.QuickEntryService) {
	validate.MustRegister()
	handler := NewHandler(service, quickEntryService)

	group := router.Group("/api/admin/v1/profile")
	group.GET("", handler.CurrentProfile)
	group.PUT("", handler.UpdateCurrentProfile)
	group.PUT("/security/password", handler.UpdatePassword)
	group.PUT("/security/email", handler.UpdateEmail)
	group.PUT("/security/phone", handler.UpdatePhone)

	quickEntry := router.Group("/api/admin/v1/users/me/quick-entries")
	quickEntry.PUT("", handler.SaveQuickEntries)
}
