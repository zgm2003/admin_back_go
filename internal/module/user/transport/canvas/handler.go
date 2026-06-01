package canvas

import (
	"admin_back_go/internal/middleware"
	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service usermodule.InitService
}

func NewHandler(service usermodule.InitService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Me(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	if identity.Platform != "" && identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	currentUser, appErr := h.service.Init(c.Request.Context(), usermodule.InitInput{
		UserID:   identity.UserID,
		Platform: enum.PlatformCanvas,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if currentUser == nil {
		response.Error(c, apperror.InternalKey("user.current_user_missing", nil, "当前用户信息未返回"))
		return
	}
	response.OK(c, currentUserFromInit(currentUser))
}
