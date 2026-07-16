package admin

import (
	"strconv"

	crontaskmodule "admin_back_go/internal/module/crontask"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service crontaskmodule.HTTPService
}

func NewHandler(service crontaskmodule.HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PageInit(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
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
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := h.service.List(c.Request.Context(), crontaskmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Title: req.Title, Name: req.Name, Status: req.Status})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.save.request.invalid", nil, "参数错误"))
		return
	}
	result, appErr := h.service.Create(c.Request.Context(), saveInputFromRequest(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Update(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.save.request.invalid", nil, "参数错误"))
		return
	}
	if appErr := h.service.Update(c.Request.Context(), id, saveInputFromRequest(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.status.invalid", nil, "无效的状态"))
		return
	}
	if appErr := h.service.ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	var req batchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.delete.empty", nil, "请选择要删除的定时任务"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Logs(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.InternalKey("crontask.service_missing", nil, "定时任务服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req logsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("crontask.logs.request.invalid", nil, "日志参数错误"))
		return
	}
	result, appErr := h.service.Logs(c.Request.Context(), crontaskmodule.LogsQuery{TaskID: id, CurrentPage: req.CurrentPage, PageSize: req.PageSize, BeforeTime: req.BeforeTime, BeforeID: req.BeforeID, Status: req.Status, StartDate: req.StartDate, EndDate: req.EndDate})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func saveInputFromRequest(req saveRequest) crontaskmodule.SaveInput {
	return crontaskmodule.SaveInput{Name: req.Name, Title: req.Title, Description: req.Description, Cron: req.Cron, CronReadable: req.CronReadable, Handler: req.Handler, Status: req.Status}
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("crontask.id.invalid", nil, "无效的定时任务ID"))
		return 0, false
	}
	return id, true
}
