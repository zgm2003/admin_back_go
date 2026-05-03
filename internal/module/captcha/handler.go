package captcha

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

// HTTPService is the handler-facing CAPTCHA service boundary.
type HTTPService interface {
	Generate(ctx context.Context) (*ChallengeResponse, *apperror.Error)
}

// Handler exposes CAPTCHA REST endpoints.
type Handler struct {
	service HTTPService
}

// NewHandler creates a CAPTCHA HTTP handler.
func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

// Generate returns a public slide CAPTCHA challenge.
func (h *Handler) Generate(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("验证码服务未配置"))
		return
	}
	result, appErr := h.service.Generate(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
