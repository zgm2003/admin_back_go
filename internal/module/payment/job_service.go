package payment

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
)

const defaultPaymentJobLimit = 50

type SyncPendingOrderInput struct {
	Limit int
}

type SyncPendingOrderResult struct {
	Scanned int
	Paid    int
	Closed  int
	Waiting int
	Failed  int
}

type CloseExpiredOrderInput struct {
	Limit int
}

type CloseExpiredOrderResult struct {
	Scanned int
	Paid    int
	Closed  int
	Waiting int
	Failed  int
}

func (s *Service) SyncPendingOrders(ctx context.Context, input SyncPendingOrderInput) (*SyncPendingOrderResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	limit := normalizePaymentJobLimit(input.Limit)
	rows, err := repo.ListPendingPayingOrders(ctx, s.now().Add(-30*time.Second), limit)
	if err != nil {
		return nil, err
	}
	result := &SyncPendingOrderResult{Scanned: len(rows)}
	for idx := range rows {
		outcome, err := s.syncPendingOrder(ctx, rows[idx], finalizeSourceCronSync)
		if err != nil {
			result.Failed++
			slog.WarnContext(ctx, "payment sync pending order failed", "order_id", rows[idx].ID, "order_no", rows[idx].OrderNo, "error", err)
			continue
		}
		applyPaymentJobOutcome(result, outcome)
	}
	remaining := limit - len(rows)
	if remaining <= 0 {
		return result, nil
	}
	uncreditedRows, err := repo.ListUncreditedPaidRecharges(ctx, remaining)
	if err != nil {
		return nil, err
	}
	result.Scanned += len(uncreditedRows)
	for idx := range uncreditedRows {
		outcome, err := s.creditUncreditedPaidRecharge(ctx, uncreditedRows[idx])
		if err != nil {
			result.Failed++
			slog.WarnContext(ctx, "payment credit paid recharge failed", "recharge_id", uncreditedRows[idx].ID, "payment_order_id", uncreditedRows[idx].PaymentOrderID, "error", err)
			continue
		}
		applyPaymentJobOutcome(result, outcome)
	}
	return result, nil
}

func (s *Service) CloseExpiredOrders(ctx context.Context, input CloseExpiredOrderInput) (*CloseExpiredOrderResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	limit := normalizePaymentJobLimit(input.Limit)
	rows, err := repo.ListExpiredOpenOrders(ctx, s.now(), limit)
	if err != nil {
		return nil, err
	}
	result := &CloseExpiredOrderResult{Scanned: len(rows)}
	for idx := range rows {
		outcome, err := s.closeExpiredOrder(ctx, rows[idx])
		if err != nil {
			result.Failed++
			slog.WarnContext(ctx, "payment close expired order failed", "order_id", rows[idx].ID, "order_no", rows[idx].OrderNo, "error", err)
			continue
		}
		applyPaymentJobOutcome(result, outcome)
	}
	return result, nil
}

type paymentJobOutcome string

const (
	paymentJobOutcomePaid    paymentJobOutcome = "paid"
	paymentJobOutcomeClosed  paymentJobOutcome = "closed"
	paymentJobOutcomeWaiting paymentJobOutcome = "waiting"
)

func (s *Service) syncPendingOrder(ctx context.Context, row Order, source string) (paymentJobOutcome, error) {
	cfg, appErr := s.configByOrder(ctx, &row)
	if appErr != nil {
		return "", appErr
	}
	platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
	if appErr != nil {
		return "", appErr
	}
	gw, appErr := s.requireGateway()
	if appErr != nil {
		return "", appErr
	}
	result, err := gw.Query(ctx, platformCfg, row.OrderNo)
	if err != nil {
		if isAlipayTradeNotExistError(err) && orderExpired(row, s.now()) {
			repo, appErr := s.requireRepository()
			if appErr != nil {
				return "", appErr
			}
			if err := closeOrderAndLinkedRecharge(ctx, repo, row.ID, s.now()); err != nil {
				return "", err
			}
			return paymentJobOutcomeClosed, nil
		}
		return "", err
	}
	switch strings.TrimSpace(resultStatus(result)) {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		paidAt := s.now()
		if result != nil && result.PaidAt != nil {
			paidAt = *result.PaidAt
		}
		if _, appErr := s.FinalizeOrderPaid(ctx, row.ID, resultTradeNo(result), paidAt, source); appErr != nil {
			return "", appErr
		}
		return paymentJobOutcomePaid, nil
	case "TRADE_CLOSED":
		repo, appErr := s.requireRepository()
		if appErr != nil {
			return "", appErr
		}
		if err := closeOrderAndLinkedRecharge(ctx, repo, row.ID, s.now()); err != nil {
			return "", err
		}
		return paymentJobOutcomeClosed, nil
	case "WAIT_BUYER_PAY":
		return paymentJobOutcomeWaiting, nil
	default:
		return "", apperror.BadRequest("未知的支付宝订单状态")
	}
}

func (s *Service) closeExpiredOrder(ctx context.Context, row Order) (paymentJobOutcome, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return "", appErr
	}
	switch row.Status {
	case orderStatusPending:
		if err := closeOrderAndLinkedRecharge(ctx, repo, row.ID, s.now()); err != nil {
			return "", err
		}
		return paymentJobOutcomeClosed, nil
	case orderStatusPaying:
		outcome, err := s.syncPendingOrder(ctx, row, finalizeSourceCronClose)
		if err != nil {
			return "", err
		}
		if outcome == paymentJobOutcomeWaiting {
			cfg, appErr := s.configByOrder(ctx, &row)
			if appErr != nil {
				return "", appErr
			}
			platformCfg, appErr := s.gatewayConfigFromConfig(*cfg)
			if appErr != nil {
				return "", appErr
			}
			gw, appErr := s.requireGateway()
			if appErr != nil {
				return "", appErr
			}
			if err := gw.Close(ctx, platformCfg, row.OrderNo); err != nil {
				return "", err
			}
			if err := closeOrderAndLinkedRecharge(ctx, repo, row.ID, s.now()); err != nil {
				return "", err
			}
			return paymentJobOutcomeClosed, nil
		}
		return outcome, nil
	default:
		return paymentJobOutcomeWaiting, nil
	}
}

func (s *Service) creditUncreditedPaidRecharge(ctx context.Context, row RechargeWithOrder) (paymentJobOutcome, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return "", appErr
	}
	paidAt := s.now()
	if row.PaidAt != nil {
		paidAt = *row.PaidAt
	} else if row.OrderPaidAt != nil {
		paidAt = *row.OrderPaidAt
	}
	if _, _, err := repo.CreditRecharge(ctx, row.ID, paidAt, s.now()); err != nil {
		return "", err
	}
	return paymentJobOutcomePaid, nil
}

func normalizePaymentJobLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return defaultPaymentJobLimit
	}
	return limit
}

func applyPaymentJobOutcome(result any, outcome paymentJobOutcome) {
	switch row := result.(type) {
	case *SyncPendingOrderResult:
		switch outcome {
		case paymentJobOutcomePaid:
			row.Paid++
		case paymentJobOutcomeClosed:
			row.Closed++
		case paymentJobOutcomeWaiting:
			row.Waiting++
		}
	case *CloseExpiredOrderResult:
		switch outcome {
		case paymentJobOutcomePaid:
			row.Paid++
		case paymentJobOutcomeClosed:
			row.Closed++
		case paymentJobOutcomeWaiting:
			row.Waiting++
		}
	}
}

func closeOrderAndLinkedRecharge(ctx context.Context, repo Repository, orderID int64, closedAt time.Time) *apperror.Error {
	if err := repo.UpdateOrderClosed(ctx, orderID, closedAt); err != nil {
		if errors.Is(err, ErrPaymentStateChanged) {
			return nil
		}
		return apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "保存支付订单关闭状态失败", err)
	}
	order, err := repo.GetOrder(ctx, orderID)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付订单失败", err)
	}
	if order == nil || order.Status != orderStatusClosed {
		return nil
	}
	recharge, err := repo.GetRechargeByOrderID(ctx, orderID)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "payment.job.linked_recharge.query_failed", nil, "查询支付订单关联充值单失败", err)
	}
	if recharge == nil || !canCloseLinkedRecharge(recharge.Status) {
		return nil
	}
	if err := repo.UpdateRechargeClosed(ctx, recharge.ID); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "payment.job.linked_recharge.close_failed", nil, "关闭支付订单关联充值单失败", err)
	}
	return nil
}

func canCloseLinkedRecharge(status string) bool {
	for _, allowed := range rechargeClosedCASStatuses {
		if status == allowed {
			return true
		}
	}
	return false
}
