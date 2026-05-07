package paynotifylog

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)
	group := router.Group("/api/admin/v1/pay-notify-logs")
	group.GET("/page-init", handler.Init)
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
}

func (h *Handler) Init(c *gin.Context) {
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付回调日志列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage:   req.CurrentPage,
		PageSize:      req.PageSize,
		TransactionNo: req.TransactionNo,
		Channel:       req.Channel,
		NotifyType:    req.NotifyType,
		ProcessStatus: req.ProcessStatus,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
	})
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

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的回调日志ID"))
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
	return nil, apperror.Internal("支付回调日志服务未配置")
}

func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付回调日志服务未配置")
}

func (nilHTTPService) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("支付回调日志服务未配置")
}
