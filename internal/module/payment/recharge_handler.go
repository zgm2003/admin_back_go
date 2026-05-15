package payment

import (
	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

func (h *Handler) RechargeInit(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().RechargeInit(c.Request.Context(), userID)
	writeResult(c, result, appErr)
}

func (h *Handler) ListRecharges(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	var req listRechargesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值记录列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListRecharges(c.Request.Context(), RechargeListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		UserID:      userID,
		Keyword:     req.Keyword,
		Status:      req.Status,
		DateStart:   req.DateStart,
		DateEnd:     req.DateEnd,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) GetRecharge(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	id, ok := routeInt64(c, "id", "无效的充值单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().GetRecharge(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) CreateRecharge(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	var req createRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("充值参数错误"))
		return
	}
	result, appErr := h.requireService().CreateRecharge(c.Request.Context(), RechargeCreateInput{
		UserID:      userID,
		PackageCode: req.PackageCode,
		PayMethod:   req.PayMethod,
		ReturnURL:   req.ReturnURL,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) PayRecharge(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	id, ok := routeInt64(c, "id", "无效的充值单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().PayRecharge(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) SyncRecharge(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	id, ok := routeInt64(c, "id", "无效的充值单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().SyncRecharge(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) CloseRecharge(c *gin.Context) {
	userID, ok := rechargeUserID(c)
	if !ok {
		return
	}
	id, ok := routeInt64(c, "id", "无效的充值单ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().CloseRecharge(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func rechargeUserID(c *gin.Context) (int64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return 0, false
	}
	return identity.UserID, true
}
