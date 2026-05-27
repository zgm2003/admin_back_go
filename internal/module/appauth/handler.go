package appauth

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type UserService interface {
	Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error)
	Profile(ctx context.Context, userID int64, currentUserID int64) (*user.ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input user.UpdateProfileInput) *apperror.Error
}

type UploadTokenService interface {
	Create(ctx context.Context, input uploadtoken.CreateInput) (*uploadtoken.CreateResponse, *apperror.Error)
}

type Handler struct {
	userService        UserService
	uploadTokenService UploadTokenService
}

func NewHandler(userService UserService, uploadTokenService UploadTokenService) *Handler {
	return &Handler{userService: userService, uploadTokenService: uploadTokenService}
}

func (h *Handler) Me(c *gin.Context) {
	currentUser, ok := h.currentUser(c)
	if !ok {
		return
	}
	response.OK(c, userFromInit(currentUser))
}

func (h *Handler) Profile(c *gin.Context) {
	identity, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	if h.userService == nil {
		response.Error(c, apperror.InternalKey("appauth.user.service_missing", nil, "用户服务未配置"))
		return
	}
	result, appErr := h.userService.Profile(c.Request.Context(), identity.UserID, identity.UserID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, profileFromUserProfile(result))
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	identity, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	if h.userService == nil {
		response.Error(c, apperror.InternalKey("appauth.user.service_missing", nil, "用户服务未配置"))
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AddressID == nil {
		response.Error(c, apperror.BadRequestKey("appauth.profile.request.invalid", nil, "个人资料参数错误"))
		return
	}
	if appErr := h.userService.UpdateProfile(c.Request.Context(), user.UpdateProfileInput{
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
	result, appErr := h.userService.Profile(c.Request.Context(), identity.UserID, identity.UserID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, profileUpdateResponse{User: userFromProfile(result)})
}

func (h *Handler) CreateUploadToken(c *gin.Context) {
	if _, ok := h.currentIdentity(c); !ok {
		return
	}
	if h.uploadTokenService == nil {
		response.Error(c, apperror.InternalKey("uploadtoken.service_missing", nil, "上传运行时服务未配置"))
		return
	}
	var req uploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("uploadtoken.request.invalid", nil, "上传 token 参数错误"))
		return
	}
	result, appErr := h.uploadTokenService.Create(c.Request.Context(), uploadtoken.CreateInput{
		Folder:   req.Folder,
		FileName: req.FileName,
		FileSize: req.FileSize,
		FileKind: req.FileKind,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) currentUser(c *gin.Context) (*user.InitResponse, bool) {
	identity, ok := h.currentIdentity(c)
	if !ok {
		return nil, false
	}
	if h.userService == nil {
		response.Error(c, apperror.InternalKey("appauth.user.service_missing", nil, "用户服务未配置"))
		return nil, false
	}
	currentUser, appErr := h.userService.Init(c.Request.Context(), user.InitInput{UserID: identity.UserID, Platform: enum.PlatformApp})
	if appErr != nil {
		response.Error(c, appErr)
		return nil, false
	}
	return currentUser, true
}

func (h *Handler) currentIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	return identity, true
}

func userFromInit(currentUser *user.InitResponse) appUser {
	if currentUser == nil {
		return appUser{}
	}
	return appUser{
		ID:       currentUser.UserID,
		Nickname: currentUser.Username,
		Avatar:   currentUser.Avatar,
	}
}
