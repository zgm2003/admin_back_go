package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"admin_back_go/internal/middleware"
	redeemcode "admin_back_go/internal/module/payment/redeemcode"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const (
	managementBodyLimit = 64 << 10
	sensitiveBodyLimit  = 1 << 10
)

var listQueryKeys = map[string]struct{}{
	"current_page": {}, "page_size": {}, "batch_no": {}, "state": {},
	"used_by": {}, "used_user": {}, "created_by": {}, "note": {},
	"created_from": {}, "created_to": {}, "expires_from": {}, "expires_to": {},
}

type HTTPService interface {
	PageInit(context.Context) (*redeemcode.PageInitResponse, *apperror.Error)
	List(context.Context, redeemcode.ListQuery) (*redeemcode.ListResponse, *apperror.Error)
	Lookup(context.Context, redeemcode.LookupInput) (*redeemcode.LookupResponse, *apperror.Error)
	Export(context.Context, redeemcode.ExportInput) (*redeemcode.ExportResponse, *apperror.Error)
	GenerateBatch(context.Context, int64, redeemcode.GenerateBatchInput) (*redeemcode.GenerateBatchResponse, *apperror.Error)
	Void(context.Context, redeemcode.VoidInput) (*redeemcode.VoidResponse, *apperror.Error)
	Redeem(context.Context, int64, string, string) (*redeemcode.RedemptionResponse, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, err := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, err, false)
}

func (h *Handler) List(c *gin.Context) {
	var request listRequest
	if !validListQueryKeys(c) || c.ShouldBindQuery(&request) != nil {
		response.Error(c, managementRequestInvalid())
		return
	}
	result, err := h.requireService().List(c.Request.Context(), listQuery(request))
	writeResult(c, result, err, true)
}

func validListQueryKeys(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	for key, values := range c.Request.URL.Query() {
		if _, allowed := listQueryKeys[key]; !allowed || len(values) != 1 {
			return false
		}
	}
	return true
}

func (h *Handler) Lookup(c *gin.Context) {
	var request lookupRequest
	if !bindJSON(c, sensitiveBodyLimit, &request) {
		response.Error(c, managementRequestInvalid())
		return
	}
	result, err := h.requireService().Lookup(c.Request.Context(), redeemcode.LookupInput{Code: string(request.Code)})
	writeResult(c, result, err, true)
}

func (h *Handler) Export(c *gin.Context) {
	var request exportRequest
	if !bindJSON(c, managementBodyLimit, &request) {
		response.Error(c, managementRequestInvalid())
		return
	}
	result, err := h.requireService().Export(c.Request.Context(), exportInput(request))
	writeResult(c, result, err, true)
}

func (h *Handler) GenerateBatch(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	var request generateBatchRequest
	if !bindJSON(c, managementBodyLimit, &request) {
		response.Error(c, managementRequestInvalid())
		return
	}
	result, err := h.requireService().GenerateBatch(c.Request.Context(), identity.UserID, redeemcode.GenerateBatchInput{
		RequestID: request.RequestID,
		Amount:    request.Amount,
		Quantity:  request.Quantity,
		ExpiresAt: request.ExpiresAt,
		Note:      request.Note,
	})
	writeResult(c, result, err, true)
}

func (h *Handler) Void(c *gin.Context) {
	var request voidRequest
	if !bindJSON(c, managementBodyLimit, &request) {
		response.Error(c, managementRequestInvalid())
		return
	}
	result, err := h.requireService().Void(c.Request.Context(), redeemcode.VoidInput{IDs: request.IDs})
	writeResult(c, result, err, false)
}

func (h *Handler) Redeem(c *gin.Context) {
	identity, ok := currentIdentity(c)
	if !ok {
		return
	}
	var request redemptionRequest
	if !bindJSON(c, sensitiveBodyLimit, &request) {
		response.Error(c, walletUnavailable())
		return
	}
	result, err := h.requireService().Redeem(c.Request.Context(), identity.UserID, identity.Platform, string(request.Code))
	if err != nil {
		err = canonicalWalletError(err)
		if retry := retryAfterSeconds(err); retry > 0 {
			c.Header("Retry-After", strconv.Itoa(retry))
		}
		response.Error(c, err)
		return
	}
	setNoStore(c)
	response.OK(c, redemptionView(result))
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return missingService{}
	}
	return h.service
}

func bindJSON(c *gin.Context, limit int64, target any) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	if err != nil {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func currentIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 || identity.Platform == "" {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	return identity, true
}

func listQuery(r listRequest) redeemcode.ListQuery {
	return redeemcode.ListQuery{
		CurrentPage: r.CurrentPage, PageSize: r.PageSize, BatchNo: r.BatchNo, State: r.State,
		UsedBy: r.UsedBy, UsedUser: r.UsedUser, CreatedBy: r.CreatedBy, Note: r.Note,
		CreatedFrom: r.CreatedFrom, CreatedTo: r.CreatedTo, ExpiresFrom: r.ExpiresFrom, ExpiresTo: r.ExpiresTo,
	}
}

func exportInput(r exportRequest) redeemcode.ExportInput {
	return redeemcode.ExportInput{
		BatchNo: r.BatchNo, State: r.State, UsedBy: r.UsedBy, UsedUser: r.UsedUser,
		CreatedBy: r.CreatedBy, Note: r.Note, CreatedFrom: r.CreatedFrom, CreatedTo: r.CreatedTo,
		ExpiresFrom: r.ExpiresFrom, ExpiresTo: r.ExpiresTo,
	}
}

func writeResult(c *gin.Context, result any, err *apperror.Error, noStore bool) {
	if err != nil {
		response.Error(c, err)
		return
	}
	if noStore {
		setNoStore(c)
	}
	response.OK(c, result)
}

func setNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
}

func redemptionView(result *redeemcode.RedemptionResponse) any {
	if result == nil {
		return nil
	}
	transaction := result.Transaction
	return redemptionResponse{
		Amount: result.Amount, Wallet: result.Wallet, Replayed: result.Replayed,
		Transaction: redemptionTransaction{
			TransactionNo: transaction.TransactionNo, Direction: transaction.Direction, DirectionText: transaction.DirectionText,
			AmountCents: transaction.AmountCents, AmountText: transaction.AmountText,
			BalanceBeforeCents: transaction.BalanceBeforeCents, BalanceBeforeText: transaction.BalanceBeforeText,
			BalanceAfterCents: transaction.BalanceAfterCents, BalanceAfterText: transaction.BalanceAfterText,
			SourceType: transaction.SourceType, SourceTypeText: transaction.SourceTypeText, CreatedAt: transaction.CreatedAt,
		},
	}
}

func managementRequestInvalid() *apperror.Error {
	return apperror.New(redeemcode.ErrorRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, redeemcode.ErrorRequestInvalid, nil, "兑换码请求参数错误")
}

func walletUnavailable() *apperror.Error {
	return apperror.New(redeemcode.ErrorWalletUnavailable, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, redeemcode.ErrorWalletUnavailable, nil, "兑换码不可用")
}

func canonicalWalletError(err *apperror.Error) *apperror.Error {
	if err == nil {
		return nil
	}
	switch {
	case err.Code == redeemcode.ErrorWalletCodeRequired:
		return apperror.New(redeemcode.ErrorWalletCodeRequired, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, redeemcode.ErrorWalletCodeRequired, nil, "请输入兑换码")
	case err.Code == redeemcode.ErrorServiceMissing:
		return apperror.New(redeemcode.ErrorWalletDependencyUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, redeemcode.ErrorWalletDependencyUnavailable, nil, "兑换服务暂不可用")
	case err.Category == apperror.CategoryRateLimit:
		return apperror.New(redeemcode.ErrorWalletRateLimited, apperror.CategoryRateLimit, http.StatusTooManyRequests, apperror.Retryable, redeemcode.ErrorWalletRateLimited, err.TemplateData, "兑换请求过于频繁")
	case err.Code == redeemcode.ErrorWalletRateLimitUnavailable:
		return apperror.New(redeemcode.ErrorWalletRateLimitUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, redeemcode.ErrorWalletRateLimitUnavailable, nil, "兑换限流服务暂不可用")
	case err.Code == redeemcode.ErrorWalletDependencyUnavailable:
		return apperror.New(redeemcode.ErrorWalletDependencyUnavailable, apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, redeemcode.ErrorWalletDependencyUnavailable, nil, "兑换服务暂不可用")
	case err.Code == redeemcode.ErrorWalletIntegrityViolation:
		return apperror.New(redeemcode.ErrorWalletIntegrityViolation, apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent, redeemcode.ErrorWalletIntegrityViolation, nil, "兑换事实完整性异常")
	default:
		return walletUnavailable()
	}
}

func retryAfterSeconds(err *apperror.Error) int {
	if err == nil || err.TemplateData == nil {
		return 0
	}
	value, ok := err.TemplateData["retry_after"]
	if !ok {
		return 0
	}
	switch value := value.(type) {
	case int:
		if value > 0 {
			return value
		}
	case int64:
		if value > 0 && value <= int64(^uint(0)>>1) {
			return int(value)
		}
	case float64:
		if value > 0 && value == float64(int(value)) {
			return int(value)
		}
	}
	return 0
}

type missingService struct{}

func (missingService) PageInit(context.Context) (*redeemcode.PageInitResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) List(context.Context, redeemcode.ListQuery) (*redeemcode.ListResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) Lookup(context.Context, redeemcode.LookupInput) (*redeemcode.LookupResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) Export(context.Context, redeemcode.ExportInput) (*redeemcode.ExportResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) GenerateBatch(context.Context, int64, redeemcode.GenerateBatchInput) (*redeemcode.GenerateBatchResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) Void(context.Context, redeemcode.VoidInput) (*redeemcode.VoidResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func (missingService) Redeem(context.Context, int64, string, string) (*redeemcode.RedemptionResponse, *apperror.Error) {
	return nil, missingServiceError()
}

func missingServiceError() *apperror.Error {
	return apperror.New(redeemcode.ErrorServiceMissing, apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent, redeemcode.ErrorServiceMissing, nil, "兑换码服务未配置")
}
