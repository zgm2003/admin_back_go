package user

import (
	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

func (h *Handler) AppMe(c *gin.Context) {
	identity, ok := h.appIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	currentUser, appErr := h.service.Init(c.Request.Context(), InitInput{UserID: identity.UserID, Platform: enum.PlatformApp})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, appUserFromInit(currentUser))
}

func (h *Handler) AppProfile(c *gin.Context) {
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
	response.OK(c, appProfileFromUserProfile(result))
}

func (h *Handler) AppUpdateProfile(c *gin.Context) {
	identity, ok := h.appIdentity(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.InternalKey("user.service_missing", nil, "用户管理服务未配置"))
		return
	}
	var req appUpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AddressID == nil {
		response.Error(c, apperror.BadRequestKey("app.profile.request.invalid", nil, "个人资料参数错误"))
		return
	}
	if appErr := h.service.UpdateProfile(c.Request.Context(), UpdateProfileInput{
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
