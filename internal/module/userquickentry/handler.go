package userquickentry

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Save(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}

	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("userquickentry.request.invalid", nil, "快捷入口参数错误"))
		return
	}

	result, appErr := h.requireService().Save(c.Request.Context(), identity.UserID, SaveInput{PermissionIDs: req.PermissionIDs})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

type nilHTTPService struct{}

func (nilHTTPService) Save(ctx context.Context, userID int64, input SaveInput) (*SaveResponse, *apperror.Error) {
	return nil, apperror.InternalKey("userquickentry.service_missing", nil, "快捷入口服务未配置")
}
