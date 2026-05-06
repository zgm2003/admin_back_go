package payruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	payalipay "admin_back_go/internal/platform/payment/alipay"
)

func TestCreateRechargeOrderRejectsAmountNotInPresets(t *testing.T) {
	service := NewService(Dependencies{Repository: &fakeRepository{}, Now: fixedNow})

	_, appErr := service.CreateRechargeOrder(context.Background(), 1, RechargeOrderCreateInput{
		Amount:    1234,
		PayMethod: "web",
		ChannelID: 1,
	})

	if appErr == nil {
		t.Fatalf("expected app error")
	}
}

func TestCreateRechargeOrderRejectsExistingNonExpiredPendingOrder(t *testing.T) {
	repo := &fakeRepository{
		channel: &Channel{ID: 1, Channel: 2, Status: 1},
		ongoingOrder: &Order{
			ID:         10,
			UserID:     1,
			PayStatus:  1,
			ExpireTime: fixedNow().Add(time.Minute),
		},
	}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	_, appErr := service.CreateRechargeOrder(context.Background(), 1, RechargeOrderCreateInput{
		Amount:    1000,
		PayMethod: "web",
		ChannelID: 1,
	})

	if appErr == nil {
		t.Fatalf("expected app error")
	}
	if repo.createdRechargeOrder {
		t.Fatalf("must not create recharge order when blocking order exists")
	}
}

func TestCreatePayAttemptRejectsOrderOwnedByAnotherUser(t *testing.T) {
	repo := &fakeRepository{orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 99, PayStatus: 1, ChannelID: 1, PayMethod: "web", PayAmount: 1000}}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	_, appErr := service.CreatePayAttempt(context.Background(), 1, "R1", PayAttemptCreateInput{PayMethod: "web"})

	if appErr == nil {
		t.Fatalf("expected app error")
	}
}

func TestCreatePayAttemptClosesPreviousActiveTransactionBeforeCreatingNewOne(t *testing.T) {
	repo := &fakeRepository{
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 1, PayStatus: 1, ChannelID: 1, PayMethod: "web", PayAmount: 1000, Title: "钱包充值 10 元"},
		channel:        activeAlipayChannel(),
		lastTxn:        &PayTransaction{ID: 20, AttemptNo: 1, Status: 2},
		createdTxn:     &PayTransaction{ID: 21, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", AttemptNo: 2, ChannelID: 1, Channel: 2, PayMethod: "web", Amount: 1000, Status: 1},
	}
	gateway := &fakeGateway{createResp: payCreateResponse()}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("T1"), Locker: noopLocker{}, Now: fixedNow})

	_, appErr := service.CreatePayAttempt(context.Background(), 1, "R1", PayAttemptCreateInput{PayMethod: "web"})

	if appErr != nil {
		t.Fatalf("CreatePayAttempt returned error: %v", appErr)
	}
	if !repo.closedPreviousTxn || !repo.createdTransaction || repo.createdTransactionAttemptNo != 2 {
		t.Fatalf("expected previous txn closed and new attempt 2, repo=%#v", repo)
	}
	if !repo.markedWaiting {
		t.Fatalf("expected transaction marked waiting")
	}
}

func TestCreatePayAttemptMarksTransactionFailedWhenGatewayReturnsError(t *testing.T) {
	repo := &fakeRepository{
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 1, PayStatus: 1, ChannelID: 1, PayMethod: "web", PayAmount: 1000, Title: "钱包充值 10 元"},
		channel:        activeAlipayChannel(),
		createdTxn:     &PayTransaction{ID: 21, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", AttemptNo: 1, ChannelID: 1, Channel: 2, PayMethod: "web", Amount: 1000, Status: 1},
	}
	gateway := &fakeGateway{createErr: errors.New("gateway down")}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("T1"), Locker: noopLocker{}, Now: fixedNow})

	_, appErr := service.CreatePayAttempt(context.Background(), 1, "R1", PayAttemptCreateInput{PayMethod: "web"})

	if appErr == nil {
		t.Fatalf("expected app error")
	}
	if !repo.markedFailed {
		t.Fatalf("expected transaction marked failed")
	}
}

func TestHandleAlipayNotifyReturnsFailOnInvalidSignature(t *testing.T) {
	repo := &fakeRepository{}
	gateway := &fakeGateway{verifyErr: errors.New("bad signature")}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, Locker: noopLocker{}, Now: fixedNow})

	body, appErr := service.HandleAlipayNotify(context.Background(), AlipayNotifyInput{Form: map[string]string{"out_trade_no": "T1"}, IP: "127.0.0.1"})

	if appErr == nil || body != "fail" {
		t.Fatalf("expected fail with app error, got body=%q err=%v", body, appErr)
	}
	if !repo.notifyFailed {
		t.Fatalf("expected notify log marked failed")
	}
}

func TestHandleAlipayNotifyReturnsFailOnAppIDOrAmountMismatch(t *testing.T) {
	tests := []struct {
		name   string
		result *payalipay.NotifyResult
	}{
		{name: "app id mismatch", result: &payalipay.NotifyResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 1000, AppID: "other"}},
		{name: "amount mismatch", result: &payalipay.NotifyResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 999, AppID: "app-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{
				notifyTxn: &PayTransaction{ID: 1, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: 2, PayMethod: "web", Amount: 1000, Status: 2},
				channel:   activeAlipayChannel(),
			}
			gateway := &fakeGateway{notifyResult: tt.result}
			service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, Locker: noopLocker{}, Now: fixedNow})

			body, appErr := service.HandleAlipayNotify(context.Background(), AlipayNotifyInput{Form: map[string]string{"out_trade_no": "T1"}, IP: "127.0.0.1"})

			if appErr == nil || body != "fail" {
				t.Fatalf("expected fail with app error, got body=%q err=%v", body, appErr)
			}
			if repo.markedPaySuccess {
				t.Fatalf("must not mark pay success on mismatch")
			}
		})
	}
}

func TestHandleAlipayNotifyDuplicateSuccessDoesNotCreditTwice(t *testing.T) {
	repo := &fakeRepository{
		notifyTxn:        &PayTransaction{ID: 1, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: 2, PayMethod: "web", Amount: 1000, Status: 3},
		channel:          activeAlipayChannel(),
		paySuccessResult: &PaySuccessResult{AlreadySuccess: true, OrderID: 10, OrderNo: "R1", TransactionID: 1},
	}
	gateway := &fakeGateway{notifyResult: &payalipay.NotifyResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 1000, AppID: "app-id", Raw: map[string]any{"out_trade_no": "T1"}}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("D1"), Locker: noopLocker{}, Now: fixedNow})

	body, appErr := service.HandleAlipayNotify(context.Background(), AlipayNotifyInput{Form: map[string]string{"out_trade_no": "T1"}, IP: "127.0.0.1"})

	if appErr != nil || body != "success" {
		t.Fatalf("expected success, got body=%q err=%v", body, appErr)
	}
	if repo.creditCalls != 1 {
		t.Fatalf("expected one idempotent repository call, got %d", repo.creditCalls)
	}
}

func TestHandleAlipayNotifySuccessReturnsRawSuccessBody(t *testing.T) {
	repo := &fakeRepository{
		notifyTxn:        &PayTransaction{ID: 1, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: 2, PayMethod: "web", Amount: 1000, Status: 2},
		channel:          activeAlipayChannel(),
		paySuccessResult: &PaySuccessResult{OrderID: 10, OrderNo: "R1", TransactionID: 1, WalletBefore: 0, WalletAfter: 1000},
	}
	gateway := &fakeGateway{notifyResult: &payalipay.NotifyResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 1000, AppID: "app-id", Raw: map[string]any{"out_trade_no": "T1"}}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("D1"), Locker: noopLocker{}, Now: fixedNow})

	body, appErr := service.HandleAlipayNotify(context.Background(), AlipayNotifyInput{Form: map[string]string{"out_trade_no": "T1"}, IP: "127.0.0.1"})

	if appErr != nil || body != "success" {
		t.Fatalf("expected success, got body=%q err=%v", body, appErr)
	}
	if !repo.notifySuccess {
		t.Fatalf("expected notify log marked success")
	}
}
