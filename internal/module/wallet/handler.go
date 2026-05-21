package wallet

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Summary(ctx context.Context, userID int64) (*SummaryResponse, *apperror.Error)
	Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error)
	Consume(ctx context.Context, input ConsumeInput) (*ConsumeResponse, *apperror.Error)
	WalletUsersPageInit(ctx context.Context) (*WalletUsersPageInitResponse, *apperror.Error)
	WalletUsers(ctx context.Context, query WalletUserListQuery) (*WalletUserListResponse, *apperror.Error)
	LedgerPageInit(ctx context.Context) (*LedgerPageInitResponse, *apperror.Error)
	Ledger(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) Summary(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Summary(c.Request.Context(), identity.UserID)
	writeResult(c, result, appErr)
}

func (h *Handler) Transactions(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	var req transactionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("wallet.transactions.request.invalid", nil, "资金明细查询参数错误"))
		return
	}
	result, appErr := h.requireService().Transactions(c.Request.Context(), TransactionListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, UserID: identity.UserID, Keyword: req.Keyword, Direction: req.Direction, SourceType: req.SourceType, DateStart: req.DateStart, DateEnd: req.DateEnd})
	writeResult(c, result, appErr)
}

func (h *Handler) Consume(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	var req consumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("wallet.consume.request.invalid", nil, "消费参数错误"))
		return
	}
	result, appErr := h.requireService().Consume(c.Request.Context(), ConsumeInput{UserID: identity.UserID, AmountCents: req.AmountCents, SourceID: req.SourceID, Remark: req.Remark})
	writeResult(c, result, appErr)
}

func (h *Handler) WalletUsersPageInit(c *gin.Context) {
	result, appErr := h.requireService().WalletUsersPageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) WalletUsers(c *gin.Context) {
	var req walletUserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("wallet.users.request.invalid", nil, "用户钱包查询参数错误"))
		return
	}
	result, appErr := h.requireService().WalletUsers(c.Request.Context(), WalletUserListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, UserID: req.UserID})
	writeResult(c, result, appErr)
}

func (h *Handler) LedgerPageInit(c *gin.Context) {
	result, appErr := h.requireService().LedgerPageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) Ledger(c *gin.Context) {
	var req transactionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("wallet.ledger.request.invalid", nil, "资金流水查询参数错误"))
		return
	}
	result, appErr := h.requireService().Ledger(c.Request.Context(), TransactionListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, UserID: req.UserID, Keyword: req.Keyword, Direction: req.Direction, SourceType: req.SourceType, DateStart: req.DateStart, DateEnd: req.DateEnd})
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) Summary(ctx context.Context, userID int64) (*SummaryResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) Consume(ctx context.Context, input ConsumeInput) (*ConsumeResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) WalletUsersPageInit(ctx context.Context) (*WalletUsersPageInitResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) WalletUsers(ctx context.Context, query WalletUserListQuery) (*WalletUserListResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) LedgerPageInit(ctx context.Context) (*LedgerPageInitResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) Ledger(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}

func currentIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
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
