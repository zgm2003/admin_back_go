package notificationtask

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
		response.Error(c, apperror.Internal("通知任务服务未配置"))
		return
	}
	result, appErr := h.service.Init(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) StatusCount(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知任务服务未配置"))
		return
	}
	var req statusCountRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("状态统计参数错误"))
		return
	}
	result, appErr := h.service.StatusCount(c.Request.Context(), StatusCountQuery{Title: req.Title})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知任务服务未配置"))
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
		Status:      req.Status,
		Title:       req.Title,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知任务服务未配置"))
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	result, appErr := h.service.Create(c.Request.Context(), CreateInput{
		Title:      req.Title,
		Content:    req.Content,
		Type:       req.Type,
		Level:      req.Level,
		Link:       req.Link,
		Platform:   req.Platform,
		TargetType: req.TargetType,
		TargetIDs:  req.TargetIDs,
		SendAt:     req.SendAt,
		CreatedBy:  identity.UserID,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Cancel(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Cancel(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("通知任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的通知任务ID"))
		return 0, false
	}
	return id, true
}

var _ HTTPService = (*Service)(nil)
