package admin

import (
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/profile"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service profile.HTTPService
}

func NewHandler(service profile.HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CurrentProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	result, appErr := h.service.Profile(c.Request.Context(), userID, userID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) UpdateCurrentProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdateProfile(c.Request.Context(), profile.UpdateProfileInput{
		UserID:        userID,
		Username:      req.Username,
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
	response.OK(c, gin.H{})
}

func (h *Handler) UpdatePassword(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdatePassword(c.Request.Context(), profile.UpdatePasswordInput{
		UserID:          userID,
		VerifyType:      req.VerifyType,
		OldPassword:     req.OldPassword,
		Account:         req.Account,
		Code:            req.Code,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdateEmail(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdateEmail(c.Request.Context(), profile.UpdateEmailInput{
		UserID: userID,
		Email:  req.Email,
		Code:   req.Code,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdatePhone(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updatePhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdatePhone(c.Request.Context(), profile.UpdatePhoneInput{
		UserID: userID,
		Phone:  req.Phone,
		Code:   req.Code,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func currentUserID(c *gin.Context) (int64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	return identity.UserID, true
}
