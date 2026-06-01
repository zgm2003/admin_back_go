package app

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

func (h *Handler) Profile(c *gin.Context) {
	identity, ok := h.appIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	result, appErr := h.service.Profile(c.Request.Context(), identity.UserID, identity.UserID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil {
		response.Error(c, apperror.InternalKey("app.profile.result_missing", nil, "个人资料信息未返回"))
		return
	}
	response.OK(c, appProfileFromUserProfile(result))
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	identity, ok := h.appIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AddressID == nil {
		response.Error(c, apperror.BadRequestKey("app.profile.request.invalid", nil, "个人资料参数错误"))
		return
	}
	if appErr := h.service.UpdateProfile(c.Request.Context(), profile.UpdateProfileInput{
		UserID:        identity.UserID,
		Username:      req.Nickname,
		Avatar:        req.Avatar,
		Sex:           req.Sex,
		Birthday:      req.Birthday,
		AddressID:     *req.AddressID,
		DetailAddress: req.DetailAddress,
		Bio:           req.Bio,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, appErr := h.service.Profile(c.Request.Context(), identity.UserID, identity.UserID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil {
		response.Error(c, apperror.InternalKey("app.profile.result_missing", nil, "个人资料信息未返回"))
		return
	}
	response.OK(c, appProfileUpdateResponse{User: appUserFromProfile(result)})
}

func (h *Handler) appIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	if identity.Platform != "" && identity.Platform != enum.PlatformApp {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return nil, false
	}
	return identity, true
}
