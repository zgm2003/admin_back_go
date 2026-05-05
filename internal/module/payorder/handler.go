package payorder

import (
	"context"
	"strconv"
	"time"

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
	group := router.Group("/api/admin/v1/pay-orders")
	group.GET("/page-init", handler.Init)
	group.GET("/status-count", handler.StatusCount)
	group.GET("", handler.List)
	group.GET("/:id", handler.Detail)
	group.PATCH("/:id/close", handler.Close)
	group.PATCH("/:id/remark", handler.Remark)
}

func (h *Handler) Init(c *gin.Context) {
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) StatusCount(c *gin.Context) {
	var req statusCountRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("订单状态统计参数错误"))
		return
	}
	result, appErr := h.requireService().StatusCount(c.Request.Context(), StatusCountQuery{OrderNo: req.OrderNo, UserID: req.UserID})
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("订单列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		OrderType:   req.OrderType,
		PayStatus:   req.PayStatus,
		OrderNo:     req.OrderNo,
		UserID:      req.UserID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
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

func (h *Handler) Remark(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req remarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("订单备注参数错误"))
		return
	}
	if appErr := h.requireService().Remark(c.Request.Context(), id, RemarkInput{Remark: req.Remark}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Close(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req closeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("关闭订单参数错误"))
		return
	}
	if appErr := h.requireService().Close(c.Request.Context(), id, CloseInput{Reason: req.Reason, Now: time.Now()}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
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
		response.Error(c, apperror.BadRequest("无效的订单ID"))
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
	return nil, apperror.Internal("订单服务未配置")
}
func (nilHTTPService) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	return nil, apperror.Internal("订单服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("订单服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("订单服务未配置")
}
func (nilHTTPService) Remark(ctx context.Context, id int64, input RemarkInput) *apperror.Error {
	return apperror.Internal("订单服务未配置")
}
func (nilHTTPService) Close(ctx context.Context, id int64, input CloseInput) *apperror.Error {
	return apperror.Internal("订单服务未配置")
}
