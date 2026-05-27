package admin

import (
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service        authmodule.SessionService
	captchaService authmodule.CaptchaHTTPService
}

func NewHandler(service authmodule.SessionService, captchaService authmodule.CaptchaHTTPService) *Handler {
	return &Handler{service: service, captchaService: captchaService}
}

func (h *Handler) LoginConfig(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}

	result, appErr := h.service.LoginConfig(c.Request.Context(), c.GetHeader("platform"))
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

func (h *Handler) Login(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("登录参数错误"))
		return
	}
	result, appErr := h.service.Login(c.Request.Context(), authmodule.LoginInput{
		LoginAccount:  req.LoginAccount,
		LoginType:     req.LoginType,
		Password:      req.Password,
		Code:          req.Code,
		CaptchaID:     req.CaptchaID,
		CaptchaAnswer: captchaAnswerFromRequest(req.CaptchaAnswer),
		Platform:      c.GetHeader("platform"),
		DeviceID:      c.GetHeader("device-id"),
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) SendCode(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("验证码参数错误"))
		return
	}
	_, appErr := h.service.SendCode(c.Request.Context(), authmodule.SendCodeInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.verify_code.sent", nil, "验证码发送成功")
}

func (h *Handler) ForgetPassword(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	var req ForgetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("重置密码参数错误"))
		return
	}
	if appErr := h.service.ForgetPassword(c.Request.Context(), authmodule.ForgetPasswordInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Refresh(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("Token认证未配置"))
		return
	}

	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.Unauthorized("缺少刷新令牌"))
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		response.Error(c, apperror.Unauthorized("缺少刷新令牌"))
		return
	}

	result, appErr := h.service.Refresh(c.Request.Context(), session.RefreshInput{
		RefreshToken: req.RefreshToken,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Logout(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("Token认证未配置"))
		return
	}

	accessToken, tokenErr := middleware.ParseBearerToken(c.GetHeader("Authorization"))
	if tokenErr != nil {
		response.Error(c, tokenErr)
		return
	}
	if appErr := h.service.Logout(c.Request.Context(), accessToken); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.logout.success", nil, "退出成功")
}

func captchaAnswerFromRequest(req *captchaAnswerRequest) *authmodule.Answer {
	if req == nil {
		return nil
	}
	return &authmodule.Answer{X: req.X, Y: req.Y}
}
