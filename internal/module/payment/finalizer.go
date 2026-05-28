package payment

import (
	"context"
	"net/http"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
)

const (
	finalizeSourceSync      = "sync"
	finalizeSourceCallback  = "callback"
	finalizeSourceCronSync  = "cron_sync"
	finalizeSourceCronClose = "cron_close"
)

type FinalizePaidResult struct {
	OrderID          int64
	RechargeID       int64
	OrderPaid        bool
	AlreadyPaid      bool
	RechargeCredited bool
	Source           string
}

func (s *Service) FinalizeOrderPaid(ctx context.Context, orderID int64, tradeNo string, paidAt time.Time, source string) (*FinalizePaidResult, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if orderID <= 0 {
		return nil, apperror.BadRequest("无效的支付订单ID")
	}
	if paidAt.IsZero() {
		paidAt = s.now()
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = finalizeSourceSync
	}

	order, err := repo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付订单失败", err)
	}
	if order == nil {
		return nil, apperror.NotFound("支付订单不存在")
	}

	result := &FinalizePaidResult{OrderID: order.ID, Source: source}
	if order.Status == orderStatusPaid {
		result.AlreadyPaid = true
	} else {
		if err := repo.UpdateOrderPaid(ctx, order.ID, resultTradeNoFromStrings(tradeNo, order.AlipayTradeNo), paidAt); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存支付订单成功状态失败", err)
		}
		result.OrderPaid = true
	}

	recharge, err := repo.GetRechargeByOrderID(ctx, order.ID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询充值单失败", err)
	}
	if recharge == nil {
		return result, nil
	}
	result.RechargeID = recharge.ID
	if recharge.Status != rechargeStatusCredited && recharge.Status != rechargeStatusPaid {
		if err := repo.UpdateRechargePaid(ctx, recharge.ID, paidAt); err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "更新充值支付状态失败", err)
		}
	}
	if _, credited, err := repo.CreditRecharge(ctx, recharge.ID, paidAt, s.now()); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "充值入账失败", err)
	} else if credited != nil && credited.Status == rechargeStatusCredited {
		result.RechargeCredited = true
	}
	return result, nil
}

func resultTradeNoFromStrings(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
