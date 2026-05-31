package canvas

import (
	"context"

	"admin_back_go/internal/middleware"
	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error)
	Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error)
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Summary(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Summary(c.Request.Context(), identity.UserID)
	writeResult(c, result, appErr)
}

func (h *Handler) Transactions(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	var req transactionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("wallet.transactions.request.invalid", nil, "资金明细查询参数错误"))
		return
	}
	result, appErr := h.requireService().Transactions(c.Request.Context(), walletmodule.TransactionListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		UserID:      identity.UserID,
		Keyword:     req.Keyword,
		Direction:   req.Direction,
		SourceType:  req.SourceType,
		DateStart:   req.DateStart,
		DateEnd:     req.DateEnd,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) Summary(ctx context.Context, userID int64) (*walletmodule.SummaryResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}

func (failingService) Transactions(ctx context.Context, query walletmodule.TransactionListQuery) (*walletmodule.TransactionListResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}

func canvasIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	if identity.Platform != "" && identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return nil, false
	}
	return identity, true
}

func serviceNotConfigured() *apperror.Error {
	return apperror.InternalKey("wallet.service_missing", nil, "钱包服务未配置")
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
