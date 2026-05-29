package admin

import (
	"context"
	"net/http"

	queuemonitormodule "admin_back_go/internal/module/queuemonitor"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	List(ctx context.Context) ([]queuemonitormodule.QueueItem, *apperror.Error)
	FailedList(ctx context.Context, query queuemonitormodule.FailedListQuery) (*queuemonitormodule.FailedListResponse, *apperror.Error)
}

type Handler struct {
	service HTTPService
	monitor http.Handler
}

func NewHandler(service HTTPService, monitor http.Handler) *Handler {
	return &Handler{service: service, monitor: monitor}
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("queuemonitor.service_missing", nil, "队列监控服务未配置"))
		return
	}
	result, appErr := h.service.List(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) FailedList(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("queuemonitor.service_missing", nil, "队列监控服务未配置"))
		return
	}
	var req failedListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("queuemonitor.failed_query.invalid", nil, "队列失败任务参数错误"))
		return
	}
	result, appErr := h.service.FailedList(c.Request.Context(), queuemonitormodule.FailedListQuery{
		Queue:       req.Queue,
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) UI(c *gin.Context) {
	if h.monitor == nil {
		response.Error(c, apperror.InternalKey("queuemonitor.ui_unavailable", nil, queuemonitormodule.ErrUIUnavailable))
		return
	}
	h.monitor.ServeHTTP(c.Writer, c.Request)
}
