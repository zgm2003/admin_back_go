package admin

import (
	"strconv"

	"admin_back_go/internal/middleware"
	notificationmodule "admin_back_go/internal/module/notification"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service notificationmodule.HTTPService
}

func NewHandler(service notificationmodule.HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PageInit(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notification.service_missing", nil, "通知服务未配置"))
		return
	}
	result, appErr := h.service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notification.service_missing", nil, "通知服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("notification.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), notificationmodule.ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		BeforeID:    req.BeforeID,
		UserID:      identity.UserID,
		Platform:    identity.Platform,
		Keyword:     req.Keyword,
		Type:        req.Type,
		Level:       req.Level,
		IsRead:      req.IsRead,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) UnreadCount(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notification.service_missing", nil, "通知服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	result, appErr := h.service.UnreadCount(c.Request.Context(), identity)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) MarkOneRead(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	h.markRead(c, []int64{id})
}

func (h *Handler) MarkRead(c *gin.Context) {
	var req readBatchRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, apperror.BadRequestKey("notification.mark_read.request.invalid", nil, "标记已读参数错误"))
			return
		}
	}
	h.markRead(c, req.IDs)
}

func (h *Handler) DeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	h.delete(c, []int64{id})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("notification.delete.empty", nil, "请选择要删除的通知"))
		return
	}
	h.delete(c, req.IDs)
}

func (h *Handler) markRead(c *gin.Context, ids []int64) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notification.service_missing", nil, "通知服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	if appErr := h.service.MarkRead(c.Request.Context(), identity, ids); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) delete(c *gin.Context, ids []int64) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notification.service_missing", nil, "通知服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), identity, ids); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func identityFromContext(c *gin.Context) (notificationmodule.Identity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return notificationmodule.Identity{}, false
	}
	return notificationmodule.Identity{UserID: identity.UserID, Platform: identity.Platform}, true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("notification.id.invalid", nil, "无效的通知ID"))
		return 0, false
	}
	return id, true
}

var _ notificationmodule.HTTPService = (*notificationmodule.Service)(nil)
