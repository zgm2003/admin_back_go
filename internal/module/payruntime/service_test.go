package payruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/enum"
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

func TestListCurrentUserRechargeOrdersMapsRowsAndLatestTransaction(t *testing.T) {
	channelID := int64(9)
	txnNo := "T1"
	txnStatus := 2
	repo := &fakeRepository{
		currentUserOrders: []CurrentUserOrderRow{{
			ID: 1, OrderNo: "R1", Title: "钱包充值 10 元", PayAmount: 1000, PayStatus: 1, BizStatus: 1,
			CreatedAt: fixedNow(), ExpireTime: timePtr(fixedNow().Add(30 * time.Minute)), ChannelID: &channelID,
			ChannelName: "支付宝沙盒", PayMethod: "web", TransactionNo: &txnNo, TransactionStatus: &txnStatus,
		}},
		currentUserOrderTotal: 1,
	}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	got, appErr := service.ListCurrentUserRechargeOrders(context.Background(), 7, CurrentUserOrderListQuery{CurrentPage: 1, PageSize: 20})

	if appErr != nil {
		t.Fatalf("unexpected app error: %v", appErr)
	}
	if repo.lastCurrentUserOrderQuery.CurrentPage != 1 || repo.lastCurrentUserOrderQuery.PageSize != 20 {
		t.Fatalf("unexpected query: %#v", repo.lastCurrentUserOrderQuery)
	}
	if got.Page.Total != 1 || len(got.List) != 1 {
		t.Fatalf("unexpected response: %#v", got)
	}
	item := got.List[0]
	if item.OrderNo != "R1" || item.PayStatusText != "待支付" || item.BizStatusText != "初始化" || item.PayMethodText != "PC网页支付" || item.ChannelName != "支付宝沙盒" {
		t.Fatalf("unexpected order item: %#v", item)
	}
	if item.TransactionNo == nil || *item.TransactionNo != "T1" || item.TransactionStatus == nil || *item.TransactionStatus != 2 {
		t.Fatalf("unexpected transaction summary: %#v", item)
	}
}

func TestCurrentUserRuntimeRejectsOtherUsersOrder(t *testing.T) {
	repo := &fakeRepository{orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 99, OrderType: 1, PayStatus: 1}}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	queryRes, queryErr := service.QueryCurrentUserRechargeResult(context.Background(), 7, "R1")
	cancelErr := service.CancelCurrentUserRechargeOrder(context.Background(), 7, "R1", CancelOrderInput{})

	if queryErr == nil || queryRes != nil {
		t.Fatalf("expected query forbidden, got result=%#v err=%v", queryRes, queryErr)
	}
	if cancelErr == nil {
		t.Fatalf("expected cancel forbidden")
	}
	if repo.closedCurrentUserOrder {
		t.Fatalf("must not close another user's order")
	}
}

func TestCancelCurrentUserRechargeOrderClosesOrderAndActiveTransactionInOneRepositoryTx(t *testing.T) {
	repo := &fakeRepository{orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 7, OrderType: 1, PayStatus: 2}, lastTxn: &PayTransaction{ID: 20, Status: 2}}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	appErr := service.CancelCurrentUserRechargeOrder(context.Background(), 7, "R1", CancelOrderInput{Reason: "用户取消"})

	if appErr != nil {
		t.Fatalf("unexpected app error: %v", appErr)
	}
	if !repo.closedCurrentUserOrder || !repo.closedPreviousTxn || repo.closedCurrentUserReason != "用户取消" {
		t.Fatalf("expected order and active txn closed, repo=%#v", repo)
	}
}

func TestCurrentUserWalletSummaryAndBills(t *testing.T) {
	repo := &fakeRepository{
		walletSummary:    &WalletSummaryRow{Exists: true, Balance: 1000, Frozen: 100, TotalRecharge: 2000, TotalConsume: 500, CreatedAt: timePtr(fixedNow())},
		walletBills:      []WalletBillRow{{ID: 3, BizActionNo: "WALLET:RECHARGE:R1", Type: 1, AvailableDelta: 1000, BalanceAfter: 1000, Title: "充值入账", OrderNo: "R1", CreatedAt: fixedNow()}},
		walletBillsTotal: 1,
	}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	summary, summaryErr := service.CurrentUserWalletSummary(context.Background(), 7)
	bills, billsErr := service.CurrentUserWalletBills(context.Background(), 7, WalletBillsQuery{CurrentPage: 1, PageSize: 20})

	if summaryErr != nil || billsErr != nil {
		t.Fatalf("unexpected errors summary=%v bills=%v", summaryErr, billsErr)
	}
	if summary.WalletExists != 1 || summary.Balance != 1000 || summary.CreatedAt == "" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if bills.Page.Total != 1 || len(bills.List) != 1 || bills.List[0].TypeText != "充值入账" {
		t.Fatalf("unexpected bills: %#v", bills)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
func TestCloseExpiredOrdersClosesUnpaidExpiredOrderAndActiveTransaction(t *testing.T) {
	repo := &fakeRepository{
		expiredOrders: []ExpiredRechargeOrder{{ID: 10, OrderNo: "R1"}},
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 7, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaying,
			ChannelID: 1, PayMethod: "web", PayAmount: 1000},
		lastTxn: &PayTransaction{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: "web", Amount: 1000, Status: enum.PayTxnWaiting},
		channel: activeAlipayChannel(),
	}
	gateway := &fakeGateway{queryResult: &payalipay.QueryResult{OutTradeNo: "T1", TradeStatus: "WAIT_BUYER_PAY", TotalAmountCents: 1000, AppID: "app-id"}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, Locker: noopLocker{}, Now: fixedNow})

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 50})

	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if result.Scanned != 1 || result.Closed != 1 || result.Paid != 0 || result.Deferred != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !repo.closedCurrentUserOrder || !repo.closedPreviousTxn || !gateway.closeCalled {
		t.Fatalf("expected local close, txn close, and best-effort gateway close; repo=%#v gateway=%#v", repo, gateway)
	}
	if repo.markedPaySuccess {
		t.Fatalf("must not credit wallet for unpaid query")
	}
}

func TestCloseExpiredOrdersMarksPaidWhenAlipayQueryReturnsSuccess(t *testing.T) {
	repo := &fakeRepository{
		expiredOrders: []ExpiredRechargeOrder{{ID: 10, OrderNo: "R1"}},
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 7, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaying,
			ChannelID: 1, PayMethod: "web", PayAmount: 1000},
		lastTxn:          &PayTransaction{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: "web", Amount: 1000, Status: enum.PayTxnWaiting},
		channel:          activeAlipayChannel(),
		paySuccessResult: &PaySuccessResult{OrderID: 10, OrderNo: "R1", TransactionID: 20},
	}
	gateway := &fakeGateway{queryResult: &payalipay.QueryResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_SUCCESS", TotalAmountCents: 1000, AppID: "app-id", Raw: map[string]any{"trade_status": "TRADE_SUCCESS"}}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("D1"), Locker: noopLocker{}, Now: fixedNow})

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 50})

	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if result.Paid != 1 || result.Closed != 0 || result.Deferred != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !repo.markedPaySuccess || repo.closedCurrentUserOrder || gateway.closeCalled {
		t.Fatalf("expected paid path only; repo=%#v gateway=%#v", repo, gateway)
	}
}

func TestCloseExpiredOrdersDefersWhenAlipayQueryFails(t *testing.T) {
	repo := &fakeRepository{
		expiredOrders:  []ExpiredRechargeOrder{{ID: 10, OrderNo: "R1"}},
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 7, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaying, ChannelID: 1, PayMethod: "web", PayAmount: 1000},
		lastTxn:        &PayTransaction{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: "web", Amount: 1000, Status: enum.PayTxnWaiting},
		channel:        activeAlipayChannel(),
	}
	gateway := &fakeGateway{queryErr: errors.New("alipay timeout")}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, Locker: noopLocker{}, Now: fixedNow})

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 50})

	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if result.Deferred != 1 || result.Closed != 0 || result.Paid != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if repo.closedCurrentUserOrder || repo.closedPreviousTxn || repo.markedPaySuccess {
		t.Fatalf("must not mutate state on query failure; repo=%#v", repo)
	}
}

func TestCloseExpiredOrdersSkipsNonAlipayActiveTransaction(t *testing.T) {
	repo := &fakeRepository{
		expiredOrders: []ExpiredRechargeOrder{{ID: 10, OrderNo: "R1"}},
		orderForUpdate: &Order{ID: 10, OrderNo: "R1", UserID: 7, OrderType: enum.PayOrderRecharge, PayStatus: enum.PayStatusPaying,
			ChannelID: 1, PayMethod: "web", PayAmount: 1000},
		lastTxn: &PayTransaction{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelWechat, PayMethod: "web", Amount: 1000, Status: enum.PayTxnWaiting},
	}
	service := NewService(Dependencies{Repository: repo, Gateway: &fakeGateway{}, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, Locker: noopLocker{}, Now: fixedNow})

	result, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{Limit: 50})

	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if result.Skipped != 1 || result.Closed != 0 || result.Paid != 0 || result.Deferred != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if repo.closedCurrentUserOrder || repo.closedPreviousTxn || repo.markedPaySuccess {
		t.Fatalf("must not mutate non-alipay active transaction; repo=%#v", repo)
	}
}

func TestSyncPendingTransactionsMarksPaidButLeavesUnpaidAlone(t *testing.T) {
	repo := &fakeRepository{
		pendingTxns:      []PendingTransaction{{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: "web", Amount: 1000}},
		channel:          activeAlipayChannel(),
		paySuccessResult: &PaySuccessResult{OrderID: 10, OrderNo: "R1", TransactionID: 20},
	}
	gateway := &fakeGateway{queryResult: &payalipay.QueryResult{OutTradeNo: "T1", TradeNo: "A1", TradeStatus: "TRADE_FINISHED", TotalAmountCents: 1000, AppID: "app-id", Raw: map[string]any{"trade_status": "TRADE_FINISHED"}}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("D1"), Now: fixedNow})

	result, err := service.SyncPendingTransactions(context.Background(), SyncPendingTransactionInput{Limit: 100})

	if err != nil {
		t.Fatalf("SyncPendingTransactions returned error: %v", err)
	}
	if result.Scanned != 1 || result.Paid != 1 || result.Unpaid != 0 || result.Deferred != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !repo.markedPaySuccess {
		t.Fatalf("expected pay success mutation")
	}
}

func TestSyncPendingTransactionsDoesNotMutateUnpaidTransaction(t *testing.T) {
	repo := &fakeRepository{
		pendingTxns: []PendingTransaction{{ID: 20, TransactionNo: "T1", OrderID: 10, OrderNo: "R1", ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: "web", Amount: 1000}},
		channel:     activeAlipayChannel(),
	}
	gateway := &fakeGateway{queryResult: &payalipay.QueryResult{OutTradeNo: "T1", TradeStatus: "WAIT_BUYER_PAY", TotalAmountCents: 1000, AppID: "app-id"}}
	service := NewService(Dependencies{Repository: repo, Gateway: gateway, Secretbox: fakeSecretbox{}, CertResolver: fakeCertResolver{}, NumberGenerator: staticNumberGenerator("D1"), Now: fixedNow})

	result, err := service.SyncPendingTransactions(context.Background(), SyncPendingTransactionInput{Limit: 100})

	if err != nil {
		t.Fatalf("SyncPendingTransactions returned error: %v", err)
	}
	if result.Unpaid != 1 || result.Paid != 0 || result.Deferred != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if repo.markedPaySuccess || repo.closedCurrentUserOrder || repo.closedPreviousTxn {
		t.Fatalf("must not mutate unpaid pending transaction; repo=%#v", repo)
	}
}

func TestCloseExpiredOrdersUsesLegacyCutoffAndDefaultLimit(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	_, err := service.CloseExpiredOrders(context.Background(), CloseExpiredOrderInput{})

	if err != nil {
		t.Fatalf("CloseExpiredOrders returned error: %v", err)
	}
	if repo.lastExpiredLimit != 50 {
		t.Fatalf("expected default limit 50, got %d", repo.lastExpiredLimit)
	}
	expectedCutoff := fixedNow().Add(-30 * time.Minute)
	if !repo.lastExpiredCutoff.Equal(expectedCutoff) {
		t.Fatalf("expected cutoff %s, got %s", expectedCutoff, repo.lastExpiredCutoff)
	}
}

func TestSyncPendingTransactionsUsesLegacyCutoffAndDefaultLimit(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(Dependencies{Repository: repo, Now: fixedNow})

	_, err := service.SyncPendingTransactions(context.Background(), SyncPendingTransactionInput{})

	if err != nil {
		t.Fatalf("SyncPendingTransactions returned error: %v", err)
	}
	if repo.lastPendingLimit != 100 {
		t.Fatalf("expected default limit 100, got %d", repo.lastPendingLimit)
	}
	expectedCutoff := fixedNow().Add(-5 * time.Minute)
	if !repo.lastPendingCutoff.Equal(expectedCutoff) {
		t.Fatalf("expected cutoff %s, got %s", expectedCutoff, repo.lastPendingCutoff)
	}
}
