package payreconcile

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Init(c *gin.Context) {
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("对账任务列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Channel: req.Channel, Status: req.Status, BillType: req.BillType, StartDate: req.StartDate, EndDate: req.EndDate})
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Retry(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().Retry(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) File(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().File(c.Request.Context(), id, c.Param("type"))
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的对账任务ID"))
		return 0, false
	}
	return id, true
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("对账任务服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("对账任务服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("对账任务服务未配置")
}
func (nilHTTPService) Retry(ctx context.Context, id int64) *apperror.Error {
	return apperror.Internal("对账任务服务未配置")
}
func (nilHTTPService) File(ctx context.Context, id int64, fileType string) (*FileResponse, *apperror.Error) {
	return nil, apperror.Internal("对账任务服务未配置")
}
