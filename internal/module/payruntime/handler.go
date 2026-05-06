package payruntime

import (
	"context"
	"net/http"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateRechargeOrder(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	var req rechargeOrderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值订单参数错误"))
		return
	}
	result, appErr := h.requireService().CreateRechargeOrder(c.Request.Context(), identity.UserID, RechargeOrderCreateInput{
		Amount:    req.Amount,
		PayMethod: req.PayMethod,
		ChannelID: req.ChannelID,
		IP:        c.ClientIP(),
	})
	writeResult(c, result, appErr)
}

func (h *Handler) CreatePayAttempt(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	var req payAttemptCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付尝试参数错误"))
		return
	}
	result, appErr := h.requireService().CreatePayAttempt(c.Request.Context(), identity.UserID, c.Param("order_no"), PayAttemptCreateInput{
		PayMethod: req.PayMethod,
		ReturnURL: req.ReturnURL,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) ListCurrentUserRechargeOrders(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	var req currentUserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值订单列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListCurrentUserRechargeOrders(c.Request.Context(), identity.UserID, CurrentUserOrderListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize})
	writeResult(c, result, appErr)
}

func (h *Handler) QueryCurrentUserRechargeResult(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	result, appErr := h.requireService().QueryCurrentUserRechargeResult(c.Request.Context(), identity.UserID, c.Param("order_no"))
	writeResult(c, result, appErr)
}

func (h *Handler) CancelCurrentUserRechargeOrder(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	var req cancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("取消订单参数错误"))
		return
	}
	appErr := h.requireService().CancelCurrentUserRechargeOrder(c.Request.Context(), identity.UserID, c.Param("order_no"), CancelOrderInput{Reason: req.Reason})
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) CurrentUserWalletSummary(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	result, appErr := h.requireService().CurrentUserWalletSummary(c.Request.Context(), identity.UserID)
	writeResult(c, result, appErr)
}

func (h *Handler) CurrentUserWalletBills(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
		return
	}
	var req walletBillsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("钱包流水参数错误"))
		return
	}
	result, appErr := h.requireService().CurrentUserWalletBills(c.Request.Context(), identity.UserID, WalletBillsQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize})
	writeResult(c, result, appErr)
}

func (h *Handler) AlipayNotify(c *gin.Context) {
	form := make(map[string]string, len(c.Request.PostForm))
	if err := c.Request.ParseForm(); err == nil {
		for key, values := range c.Request.PostForm {
			if len(values) > 0 {
				form[key] = values[0]
			}
		}
	}
	headers := make(map[string]string, len(c.Request.Header))
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	body, _ := h.requireService().HandleAlipayNotify(c.Request.Context(), AlipayNotifyInput{
		Form:    form,
		Headers: headers,
		IP:      c.ClientIP(),
	})
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
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

func (nilHTTPService) CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) (*CurrentUserOrderListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) QueryCurrentUserRechargeResult(ctx context.Context, userID int64, orderNo string) (*OrderQueryResultResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) CancelCurrentUserRechargeOrder(ctx context.Context, userID int64, orderNo string, input CancelOrderInput) *apperror.Error {
	return apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) (*WalletBillsResponse, *apperror.Error) {
	return nil, apperror.Internal("支付运行时服务未配置")
}

func (nilHTTPService) HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error) {
	return "fail", apperror.Internal("支付运行时服务未配置")
}
