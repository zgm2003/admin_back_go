package admin

import (
	"strconv"

	"admin_back_go/internal/middleware"
	notificationtaskmodule "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type TaskHandler struct {
	service notificationtaskmodule.HTTPService
}

func NewTaskHandler(service notificationtaskmodule.HTTPService) *TaskHandler {
	return &TaskHandler{service: service}
}

func (h *TaskHandler) PageInit(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	result, appErr := h.service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *TaskHandler) StatusCount(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	var req taskStatusCountRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("notificationtask.status_count.request.invalid", nil, "状态统计参数错误"))
		return
	}
	result, appErr := h.service.StatusCount(c.Request.Context(), notificationtaskmodule.StatusCountQuery{Title: req.Title})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *TaskHandler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	var req taskListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("notificationtask.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), notificationtaskmodule.ListQuery{
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

func (h *TaskHandler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	var req taskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("notificationtask.create.request.invalid", nil, "参数错误"))
		return
	}
	result, appErr := h.service.Create(c.Request.Context(), notificationtaskmodule.CreateInput{
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

func (h *TaskHandler) Cancel(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	id, ok := taskRouteID(c)
	if !ok {
		return
	}
	if appErr := h.service.Cancel(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *TaskHandler) Delete(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("notificationtask.service_missing", nil, "通知任务服务未配置"))
		return
	}
	id, ok := taskRouteID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func taskRouteID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("notificationtask.id.invalid", nil, "无效的通知任务ID"))
		return 0, false
	}
	return id, true
}

var _ notificationtaskmodule.HTTPService = (*notificationtaskmodule.Service)(nil)
