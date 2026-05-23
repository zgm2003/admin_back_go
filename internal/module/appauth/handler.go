package appauth

import (
	"context"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type AuthService interface {
	Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type UserService interface {
	Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error)
}

type Handler struct {
	authService AuthService
	userService UserService
}

func NewHandler(authService AuthService, userService UserService) *Handler {
	return &Handler{authService: authService, userService: userService}
}

func (h *Handler) Login(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("appauth.login.service_missing", nil, "登录服务未配置"))
		return
	}
	if h.userService == nil {
		response.Error(c, apperror.InternalKey("appauth.user.service_missing", nil, "用户服务未配置"))
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("appauth.login.request.invalid", nil, "登录参数错误"))
		return
	}

	result, appErr := h.authService.Login(c.Request.Context(), auth.LoginInput{
		LoginAccount: req.Account,
		LoginType:    auth.LoginTypePassword,
		Password:     req.Password,
		Platform:     enum.PlatformApp,
		DeviceID:     c.GetHeader("device-id"),
		ClientIP:     c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || strings.TrimSpace(result.AccessToken) == "" || result.UserID <= 0 {
		response.Error(c, apperror.InternalKey("appauth.login.result_invalid", nil, "登录结果无效"))
		return
	}

	currentUser, appErr := h.userService.Init(c.Request.Context(), user.InitInput{UserID: result.UserID, Platform: enum.PlatformApp})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	response.OK(c, loginResponse{
		Token: result.AccessToken,
		User:  userFromInit(currentUser),
	})
}

func (h *Handler) Me(c *gin.Context) {
	currentUser, ok := h.currentUser(c)
	if !ok {
		return
	}
	response.OK(c, userFromInit(currentUser))
}

func (h *Handler) Logout(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("appauth.login.service_missing", nil, "登录服务未配置"))
		return
	}
	accessToken, tokenErr := middleware.ParseBearerToken(c.GetHeader("Authorization"))
	if tokenErr != nil {
		response.Error(c, tokenErr)
		return
	}
	if appErr := h.authService.Logout(c.Request.Context(), accessToken); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKNull(c)
}

func (h *Handler) currentUser(c *gin.Context) (*user.InitResponse, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
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
