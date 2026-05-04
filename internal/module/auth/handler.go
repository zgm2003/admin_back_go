package auth

import (
	"context"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type SessionService interface {
	Login(ctx context.Context, input LoginInput) (*LoginResponse, *apperror.Error)
	SendCode(ctx context.Context, input SendCodeInput) (string, *apperror.Error)
	LoginConfig(ctx context.Context, platform string) (*LoginConfigResponse, *apperror.Error)
	Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type Handler struct {
	service SessionService
}

func NewHandler(service SessionService) *Handler {
	return &Handler{service: service}
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
	result, appErr := h.service.Login(c.Request.Context(), LoginInput{
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
	message, appErr := h.service.SendCode(c.Request.Context(), SendCodeInput{
		Account: req.Account,
		Scene:   req.Scene,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessage(c, gin.H{}, message)
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
	response.OKWithMessage(c, gin.H{}, "退出成功")
}

func captchaAnswerFromRequest(req *captchaAnswerRequest) *captcha.Answer {
	if req == nil {
		return nil
	}
	return &captcha.Answer{X: req.X, Y: req.Y}
}
