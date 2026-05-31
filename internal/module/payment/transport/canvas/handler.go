package canvas

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	paymentmodule "admin_back_go/internal/module/payment"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	RechargePageInit(ctx context.Context, userID int64) (*paymentmodule.RechargePageInitResponse, *apperror.Error)
	ListRecharges(ctx context.Context, query paymentmodule.RechargeListQuery) (*paymentmodule.RechargeListResponse, *apperror.Error)
	CreateRecharge(ctx context.Context, input paymentmodule.RechargeCreateInput) (*paymentmodule.RechargePayResponse, *apperror.Error)
	PayRecharge(ctx context.Context, userID int64, id int64) (*paymentmodule.RechargePayResponse, *apperror.Error)
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RechargePageInit(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().RechargePageInit(c.Request.Context(), identity.UserID)
	writeResult(c, result, appErr)
}

func (h *Handler) ListRecharges(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	var req listRechargesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值记录列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListRecharges(c.Request.Context(), paymentmodule.RechargeListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		UserID:      identity.UserID,
		Keyword:     req.Keyword,
		Status:      req.Status,
		DateStart:   req.DateStart,
		DateEnd:     req.DateEnd,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) CreateRecharge(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	var req createRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值参数错误"))
		return
	}
	result, appErr := h.requireService().CreateRecharge(c.Request.Context(), paymentmodule.RechargeCreateInput{
		UserID:      identity.UserID,
		PackageCode: req.PackageCode,
		PayMethod:   req.PayMethod,
		ReturnURL:   req.ReturnURL,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) PayRecharge(c *gin.Context) {
	identity, ok := canvasIdentity(c)
	if !ok {
		return
	}
	id, ok := routeInt64(c, "id", "无效的充值单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().PayRecharge(c.Request.Context(), identity.UserID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

type nilHTTPService struct{}

func (nilHTTPService) RechargePageInit(ctx context.Context, userID int64) (*paymentmodule.RechargePageInitResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}

func (nilHTTPService) ListRecharges(ctx context.Context, query paymentmodule.RechargeListQuery) (*paymentmodule.RechargeListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}

func (nilHTTPService) CreateRecharge(ctx context.Context, input paymentmodule.RechargeCreateInput) (*paymentmodule.RechargePayResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}

func (nilHTTPService) PayRecharge(ctx context.Context, userID int64, id int64) (*paymentmodule.RechargePayResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
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

func routeInt64(c *gin.Context, name string, msg string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest(msg))
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
