package user

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type InitService interface {
	Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error)
}

type Handler struct {
	service InitService
}

func NewHandler(service InitService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户初始化服务未配置"))
		return
	}

	result, appErr := h.service.Init(c.Request.Context(), InitInput{
		UserID:   identity.UserID,
		Platform: identity.Platform,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
