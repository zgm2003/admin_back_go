package usersession

import (
	"context"
	"strconv"

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

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("usersession.list.request.invalid", nil, "用户会话列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Username:    req.Username,
		Platform:    req.Platform,
		Status:      req.Status,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Stats(c *gin.Context) {
	result, appErr := h.requireService().Stats(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) Revoke(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.SessionID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	result, appErr := h.requireService().Revoke(c.Request.Context(), id, identity.SessionID)
	writeResult(c, result, appErr)
}

func (h *Handler) BatchRevoke(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.SessionID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	var req batchRevokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("usersession.batch_revoke.request.invalid", nil, "批量踢下线参数错误"))
		return
	}
	result, appErr := h.requireService().BatchRevoke(c.Request.Context(), BatchRevokeInput{IDs: req.IDs}, identity.SessionID)
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("usersession.id.invalid", nil, "无效的用户会话ID"))
		return 0, false
	}
	return id, true
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilHTTPService) Stats(ctx context.Context) (*StatsResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilHTTPService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*RevokeResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilHTTPService) BatchRevoke(ctx context.Context, input BatchRevokeInput, currentSessionID int64) (*BatchRevokeResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}
