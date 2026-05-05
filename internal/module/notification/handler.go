package notification

import (
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

func (h *Handler) Init(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知服务未配置"))
		return
	}
	result, appErr := h.service.Init(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知服务未配置"))
		return
	}
	identity, ok := identityFromContext(c)
	if !ok {
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
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
		response.Error(c, apperror.Internal("通知服务未配置"))
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
			response.Error(c, apperror.BadRequest("标记已读参数错误"))
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
		response.Error(c, apperror.BadRequest("请选择要删除的通知"))
		return
	}
	h.delete(c, req.IDs)
}

func (h *Handler) markRead(c *gin.Context, ids []int64) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知服务未配置"))
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
		response.Error(c, apperror.Internal("通知服务未配置"))
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

func identityFromContext(c *gin.Context) (Identity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return Identity{}, false
	}
	return Identity{UserID: identity.UserID, Platform: identity.Platform}, true
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的通知ID"))
		return 0, false
	}
	return id, true
}

var _ HTTPService = (*Service)(nil)
