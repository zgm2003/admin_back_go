package appauth

import (
	"context"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type AuthService interface {
	LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error)
	Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error)
	SendCode(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type CaptchaService interface {
	Generate(ctx context.Context) (*captcha.ChallengeResponse, *apperror.Error)
}

type UserService interface {
	Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error)
	Profile(ctx context.Context, userID int64, currentUserID int64) (*user.ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input user.UpdateProfileInput) *apperror.Error
}

type UploadTokenService interface {
	Create(ctx context.Context, input uploadtoken.CreateInput) (*uploadtoken.CreateResponse, *apperror.Error)
}

type Handler struct {
	authService        AuthService
	captchaService     CaptchaService
	userService        UserService
	uploadTokenService UploadTokenService
}

func NewHandler(authService AuthService, captchaService CaptchaService, userService UserService, uploadTokenService UploadTokenService) *Handler {
	return &Handler{authService: authService, captchaService: captchaService, userService: userService, uploadTokenService: uploadTokenService}
}

func (h *Handler) LoginConfig(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("appauth.login.service_missing", nil, "登录服务未配置"))
		return
	}
	result, appErr := h.authService.LoginConfig(c.Request.Context(), enum.PlatformApp)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Captcha(c *gin.Context) {
	if h.captchaService == nil {
		response.Error(c, apperror.InternalKey("captcha.service_missing", nil, "验证码服务未配置"))
		return
	}
	result, appErr := h.captchaService.Generate(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) SendCode(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("appauth.login.service_missing", nil, "登录服务未配置"))
		return
	}
	var req sendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("appauth.send_code.request.invalid", nil, "验证码参数错误"))
		return
	}
	if _, appErr := h.authService.SendCode(c.Request.Context(), auth.SendCodeInput{
		Account: req.Account,
		Scene:   req.Scene,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.verify_code.sent", nil, "验证码发送成功")
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
		LoginAccount:  req.LoginAccount,
		LoginType:     req.LoginType,
		Password:      req.Password,
		Code:          req.Code,
		CaptchaID:     req.CaptchaID,
		CaptchaAnswer: captchaAnswerFromRequest(req.CaptchaAnswer),
		Platform:      enum.PlatformApp,
		DeviceID:      c.GetHeader("device-id"),
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
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
