package auth

import (
	"context"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type SessionService interface {
	Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error)
	Logout(ctx context.Context, accessToken string) *apperror.Error
}

type Handler struct {
	service SessionService
}

func NewHandler(service SessionService) *Handler {
	return &Handler{service: service}
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
