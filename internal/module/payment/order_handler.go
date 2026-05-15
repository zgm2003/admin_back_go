package payment

import (
	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

func (h *Handler) OrderInit(c *gin.Context) {
	result, appErr := h.requireService().OrderInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) ListOrders(c *gin.Context) {
	var req listOrdersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付订单列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListOrders(c.Request.Context(), OrderListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Keyword:     req.Keyword,
		ConfigCode:  req.ConfigCode,
		Provider:    req.Provider,
		PayMethod:   req.PayMethod,
		Status:      req.Status,
		DateStart:   req.DateStart,
		DateEnd:     req.DateEnd,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) GetOrder(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付订单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().GetOrder(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付订单参数错误"))
		return
	}
	result, appErr := h.requireService().CreateOrder(c.Request.Context(), OrderCreateInput{
		ConfigCode:    req.ConfigCode,
		PayMethod:     req.PayMethod,
		Subject:       req.Subject,
		AmountCents:   req.AmountCents,
		ReturnURL:     req.ReturnURL,
		ExpireMinutes: req.ExpireMinutes,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) PayOrder(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付订单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().PayOrder(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) SyncOrder(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付订单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().SyncOrder(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) CloseOrder(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付订单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().CloseOrder(c.Request.Context(), id)
	writeResult(c, result, appErr)
}
