package payment

import (
	"context"
	"net/http"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) ChannelInit(c *gin.Context) {
	result, appErr := h.requireService().ChannelInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) ListChannels(c *gin.Context) {
	var req listChannelsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付渠道列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListChannels(c.Request.Context(), ChannelListQuery{
		CurrentPage: req.CurrentPage, PageSize: req.PageSize, Name: req.Name, Provider: req.Provider, Status: req.Status,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) CreateChannel(c *gin.Context) {
	var req channelMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付渠道参数错误"))
		return
	}
	id, appErr := h.requireService().CreateChannel(c.Request.Context(), channelInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付渠道ID")
	if !ok {
		return
	}
	var req channelMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付渠道参数错误"))
		return
	}
	writeEmpty(c, h.requireService().UpdateChannel(c.Request.Context(), id, channelInput(req)))
}

func (h *Handler) ChangeChannelStatus(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付渠道ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付渠道状态参数错误"))
		return
	}
	writeEmpty(c, h.requireService().ChangeChannelStatus(c.Request.Context(), id, req.Status))
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付渠道ID")
	if !ok {
		return
	}
	writeEmpty(c, h.requireService().DeleteChannel(c.Request.Context(), id))
}

func (h *Handler) OrderInit(c *gin.Context) {
	result, appErr := h.requireService().OrderInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) ListOrders(c *gin.Context) {
	var req orderListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付订单列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListOrders(c.Request.Context(), OrderListQuery{
		CurrentPage: req.CurrentPage, PageSize: req.PageSize, OrderNo: req.OrderNo, UserID: req.UserID, Status: req.Status, StartDate: req.StartDate, EndDate: req.EndDate,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) CreateOrder(c *gin.Context) {
	identity, ok := currentUser(c)
	if !ok {
		return
	}
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付订单参数错误"))
		return
	}
	result, appErr := h.requireService().CreateOrder(c.Request.Context(), CreateOrderInput{
		UserID: identity.UserID, ChannelID: req.ChannelID, PayMethod: req.PayMethod, Subject: req.Subject, AmountCents: req.AmountCents,
		ReturnURL: req.ReturnURL, BusinessType: req.BusinessType, BusinessRef: req.BusinessRef, ClientIP: c.ClientIP(),
	})
	writeResult(c, result, appErr)
}

func (h *Handler) GetOrderResult(c *gin.Context) {
	identity, ok := currentUser(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().GetOrderResult(c.Request.Context(), identity.UserID, c.Param("order_no"))
	writeResult(c, result, appErr)
}

func (h *Handler) PayOrder(c *gin.Context) {
	identity, ok := currentUser(c)
	if !ok {
		return
	}
	var req payOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付请求参数错误"))
		return
	}
	result, appErr := h.requireService().PayOrder(c.Request.Context(), identity.UserID, c.Param("order_no"), req.ReturnURL)
	writeResult(c, result, appErr)
}

func (h *Handler) CancelOrder(c *gin.Context) {
	identity, ok := currentUser(c)
	if !ok {
		return
	}
	writeEmpty(c, h.requireService().CancelOrder(c.Request.Context(), identity.UserID, c.Param("order_no")))
}

func (h *Handler) GetAdminOrder(c *gin.Context) {
	result, appErr := h.requireService().GetAdminOrder(c.Request.Context(), c.Param("order_no"))
	writeResult(c, result, appErr)
}

func (h *Handler) CloseAdminOrder(c *gin.Context) {
	writeEmpty(c, h.requireService().CloseAdminOrder(c.Request.Context(), c.Param("order_no")))
}

func (h *Handler) ListEvents(c *gin.Context) {
	var req eventListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付事件列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListEvents(c.Request.Context(), EventListQuery{
		CurrentPage: req.CurrentPage, PageSize: req.PageSize, OrderNo: req.OrderNo, OutTradeNo: req.OutTradeNo, EventType: req.EventType, ProcessStatus: req.ProcessStatus,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) GetEvent(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付事件ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().GetEvent(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) AlipayNotify(c *gin.Context) {
	service := h.service
	if service == nil {
		c.String(http.StatusOK, "fail")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	form := make(map[string]string, len(c.Request.PostForm))
	for key, values := range c.Request.PostForm {
		if len(values) > 0 {
			form[key] = values[0]
		}
	}
	body, _ := service.HandleAlipayNotify(c.Request.Context(), NotifyInput{Form: form, IP: c.ClientIP()})
	if body == "" {
		body = "fail"
	}
	c.String(http.StatusOK, body)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func currentUser(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("未登录"))
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

func channelInput(req channelMutationRequest) ChannelMutationInput {
	return ChannelMutationInput{
		Code: req.Code, Name: req.Name, Provider: req.Provider, SupportedMethods: req.SupportedMethods,
		AppID: req.AppID, MerchantID: req.MerchantID, NotifyURL: req.NotifyURL, ReturnURL: req.ReturnURL, PrivateKey: req.PrivateKey,
		AppCertPath: req.AppCertPath, AlipayCertPath: req.AlipayCertPath, AlipayRootCertPath: req.AlipayRootCertPath,
		IsSandbox: req.IsSandbox, Status: req.Status, Remark: req.Remark,
	}
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func writeEmpty(c *gin.Context, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

type nilHTTPService struct{}

func (nilHTTPService) ChannelInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ListChannels(ctx context.Context, query ChannelListQuery) (*ChannelListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) CreateChannel(ctx context.Context, input ChannelMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) UpdateChannel(ctx context.Context, id int64, input ChannelMutationInput) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ChangeChannelStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) DeleteChannel(ctx context.Context, id int64) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) OrderInit(ctx context.Context) (*ChannelInitResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ListOrders(ctx context.Context, query OrderListQuery) (*OrderListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) GetAdminOrder(ctx context.Context, orderNo string) (*OrderDetailResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) GetOrderResult(ctx context.Context, userID int64, orderNo string) (*ResultResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) CreateOrder(ctx context.Context, input CreateOrderInput) (*CreateOrderResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) PayOrder(ctx context.Context, userID int64, orderNo string, returnURL string) (*PayOrderResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) CancelOrder(ctx context.Context, userID int64, orderNo string) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) CloseAdminOrder(ctx context.Context, orderNo string) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ListEvents(ctx context.Context, query EventListQuery) (*EventListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) GetEvent(ctx context.Context, id int64) (*EventDetailResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) HandleAlipayNotify(ctx context.Context, input NotifyInput) (string, *apperror.Error) {
	return "fail", apperror.Internal("支付服务未配置")
}
