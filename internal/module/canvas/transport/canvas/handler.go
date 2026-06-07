package canvas

import (
	"context"

	"admin_back_go/internal/middleware"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Settings(c *gin.Context) {
	var userID int64
	if identity := middleware.GetAuthIdentity(c); identity != nil {
		userID = identity.UserID
	}
	result, appErr := h.requireService().PublicSettings(c.Request.Context(), canvasmodule.SettingsInput{UserID: userID})
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.service_missing", nil, "Canvas服务未配置")
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
