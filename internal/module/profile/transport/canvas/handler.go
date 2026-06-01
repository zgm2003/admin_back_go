package canvas

import (
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service profile.AppService
}

func NewHandler(service profile.AppService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Me(c *gin.Context) {
	identity, ok := h.canvasIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	currentUser, appErr := h.service.Init(c.Request.Context(), profile.InitInput{UserID: identity.UserID, Platform: enum.PlatformCanvas})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, currentUser)
}

func (h *Handler) canvasIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	if identity.Platform != "" && identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return nil, false
	}
	return identity, true
}
