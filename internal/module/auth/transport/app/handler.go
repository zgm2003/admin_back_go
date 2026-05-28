package app

import (
	"context"
	"strings"

	"admin_back_go/internal/middleware"
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type UserInitService interface {
	Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error)
}

type Handler struct {
	platform       string
	authService    authmodule.SessionService
	captchaService authmodule.CaptchaHTTPService
	userService    UserInitService
}

func NewHandler(opts RouteOptions) *Handler {
	return &Handler{
		platform:       strings.TrimSpace(opts.Platform),
		authService:    opts.AuthService,
		captchaService: opts.CaptchaService,
		userService:    opts.UserService,
	}
}

func (h *Handler) LoginConfig(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.service_missing", nil, "登录服务未配置"))
		return
	}
	result, appErr := h.authService.LoginConfig(c.Request.Context(), h.platform)
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
		response.Error(c, apperror.UnauthorizedKey("auth.platform.service_missing", nil, "登录服务未配置"))
		return
	}
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("auth.send_code.request.invalid", nil, "验证码参数错误"))
		return
	}
	if _, appErr := h.authService.SendCode(c.Request.Context(), authmodule.SendCodeInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.verify_code.sent", nil, "验证码发送成功")
}

func (h *Handler) Login(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.service_missing", nil, "登录服务未配置"))
		return
	}
	if h.userService == nil {
		response.Error(c, apperror.InternalKey("auth.platform.user_service_missing", nil, "用户服务未配置"))
		return
	}
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("auth.login.request.invalid", nil, "登录参数错误"))
		return
	}
	result, appErr := h.authService.Login(c.Request.Context(), authmodule.LoginInput{
		LoginAccount:  req.LoginAccount,
		LoginType:     req.LoginType,
		Password:      req.Password,
		Code:          req.Code,
		CaptchaID:     req.CaptchaID,
		CaptchaAnswer: captchaAnswerFromRequest(req.CaptchaAnswer),
		Platform:      h.platform,
		DeviceID:      c.GetHeader("device-id"),
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || strings.TrimSpace(result.AccessToken) == "" || result.UserID <= 0 {
		response.Error(c, apperror.InternalKey("auth.platform_login.result_invalid", nil, "登录结果无效"))
		return
	}
	currentUser, appErr := h.userService.Init(c.Request.Context(), user.InitInput{UserID: result.UserID, Platform: h.platform})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, loginResponse{Token: result.AccessToken, User: userFromInit(currentUser)})
}

func (h *Handler) Logout(c *gin.Context) {
	if h.authService == nil {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.service_missing", nil, "登录服务未配置"))
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

func captchaAnswerFromRequest(req *captchaAnswerRequest) *authmodule.Answer {
	if req == nil {
		return nil
	}
	return &authmodule.Answer{X: req.X, Y: req.Y}
}
