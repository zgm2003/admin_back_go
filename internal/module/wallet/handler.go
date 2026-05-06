package wallet

import (
	"context"

	"admin_back_go/internal/apperror"
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
	result, appErr := h.requireService().Init(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("钱包列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		UserID:      req.UserID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Transactions(c *gin.Context) {
	var req transactionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("钱包流水参数错误"))
		return
	}
	result, appErr := h.requireService().Transactions(c.Request.Context(), TransactionListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		UserID:      req.UserID,
		Type:        req.Type,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	})
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

type nilHTTPService struct{}

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("钱包服务未配置")
}

func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("钱包服务未配置")
}

func (nilHTTPService) Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	return nil, apperror.Internal("钱包服务未配置")
}
