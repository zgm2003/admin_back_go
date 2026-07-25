package payment

import (
	"context"
	"errors"
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
	OrderID                 int64
	RechargeID              int64
	OrderPaid               bool
	AlreadyPaid             bool
	RechargeCredited        bool
	RechargeAlreadyCredited bool
	RawOrder                bool
	Source                  string
}

// PaidOrderFinalization is the committed fact returned by the atomic
// order/recharge/wallet settlement transaction.
type PaidOrderFinalization struct {
	Order                   *Order
	Recharge                *Recharge
	Wallet                  *Wallet
	OrderPaid               bool
	OrderAlreadyPaid        bool
	RechargeCredited        bool
	RechargeAlreadyCredited bool
	RawOrder                bool
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

	fact, err := repo.FinalizePaidOrder(ctx, orderID, tradeNo, paidAt, s.now())
	if err != nil {
		if errors.Is(err, ErrPaymentOrderNotFound) {
			return nil, apperror.NotFound("支付订单不存在")
		}
		if errors.Is(err, ErrPaymentStateChanged) {
			return nil, apperror.BadRequest("充值单状态已变化，不能入账")
		}
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "支付终结失败", err)
	}
	if fact == nil || fact.Order == nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "支付终结失败", ErrPaymentStateChanged)
	}
	result := &FinalizePaidResult{
		OrderID:                 fact.Order.ID,
		OrderPaid:               fact.OrderPaid,
		AlreadyPaid:             fact.OrderAlreadyPaid,
		RechargeCredited:        fact.RechargeCredited,
		RechargeAlreadyCredited: fact.RechargeAlreadyCredited,
		RawOrder:                fact.RawOrder,
		Source:                  source,
	}
	if fact.Recharge != nil {
		result.RechargeID = fact.Recharge.ID
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
