package payruntime

import (
	"context"
	"errors"
	"time"

	payalipay "admin_back_go/internal/platform/payment/alipay"
)

type fakeRepository struct {
	channel                     *Channel
	ongoingOrder                *Order
	orderForUpdate              *Order
	lastTxn                     *PayTransaction
	createdTxn                  *PayTransaction
	notifyTxn                   *PayTransaction
	paySuccessResult            *PaySuccessResult
	currentUserOrders           []CurrentUserOrderRow
	currentUserOrderTotal       int64
	lastCurrentUserOrderQuery   CurrentUserOrderListQuery
	walletSummary               *WalletSummaryRow
	walletBills                 []WalletBillRow
	walletBillsTotal            int64
	lastWalletBillsQuery        WalletBillsQuery
	createdRechargeOrder        bool
	createdTransaction          bool
	createdTransactionAttemptNo int
	closedPreviousTxn           bool
	closedCurrentUserOrder      bool
	closedCurrentUserReason     string
	markedWaiting               bool
	markedFailed                bool
	notifyFailed                bool
	notifySuccess               bool
	markedPaySuccess            bool
	creditCalls                 int
}

func (r *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(r)
}

func (r *fakeRepository) FindActiveAlipayChannel(ctx context.Context, channelID int64) (*Channel, error) {
	return r.channel, nil
}

func (r *fakeRepository) FindLatestOngoingRechargeByUser(ctx context.Context, userID int64) (*Order, error) {
	return r.ongoingOrder, nil
}

func (r *fakeRepository) CreateRechargeOrder(ctx context.Context, input RechargeOrderMutation) (*RechargeOrderCreated, error) {
	r.createdRechargeOrder = true
	return &RechargeOrderCreated{OrderID: 1, OrderNo: input.OrderNo, PayAmount: input.Amount, ExpireTime: input.ExpireTime}, nil
}

func (r *fakeRepository) ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) ([]CurrentUserOrderRow, int64, error) {
	r.lastCurrentUserOrderQuery = query
	return r.currentUserOrders, r.currentUserOrderTotal, nil
}

func (r *fakeRepository) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	return r.orderForUpdate, nil
}

func (r *fakeRepository) GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error) {
	return r.orderForUpdate, nil
}

func (r *fakeRepository) FindLastAnyTransactionForOrder(ctx context.Context, orderID int64, successTxnID int64) (*PayTransaction, error) {
	if r.lastTxn != nil {
		return r.lastTxn, nil
	}
	return r.notifyTxn, nil
}

func (r *fakeRepository) CloseCurrentUserRechargeOrder(ctx context.Context, orderID int64, currentStatus int, reason string, now time.Time) (int64, error) {
	r.closedCurrentUserOrder = true
	r.closedCurrentUserReason = reason
	return 1, nil
}

func (r *fakeRepository) CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryRow, error) {
	return r.walletSummary, nil
}

func (r *fakeRepository) CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) ([]WalletBillRow, int64, error) {
	r.lastWalletBillsQuery = query
	return r.walletBills, r.walletBillsTotal, nil
}

func (r *fakeRepository) FindLastActiveTransactionForUpdate(ctx context.Context, orderID int64) (*PayTransaction, error) {
	return r.lastTxn, nil
}

func (r *fakeRepository) CloseTransaction(ctx context.Context, txnID int64, now time.Time) error {
	r.closedPreviousTxn = true
	return nil
}

func (r *fakeRepository) CreateTransaction(ctx context.Context, input TransactionMutation) (*PayTransaction, error) {
	r.createdTransaction = true
	r.createdTransactionAttemptNo = input.AttemptNo
	if r.createdTxn != nil {
		return r.createdTxn, nil
	}
	return &PayTransaction{
		ID:            1,
		TransactionNo: input.TransactionNo,
		OrderID:       input.OrderID,
		OrderNo:       input.OrderNo,
		AttemptNo:     input.AttemptNo,
		ChannelID:     input.ChannelID,
		Channel:       input.Channel,
		PayMethod:     input.PayMethod,
		Amount:        input.Amount,
		Status:        1,
	}, nil
}

func (r *fakeRepository) MarkTransactionWaiting(ctx context.Context, txnID int64, raw map[string]any, now time.Time) error {
	r.markedWaiting = true
	return nil
}

func (r *fakeRepository) MarkTransactionFailed(ctx context.Context, txnID int64, reason string, now time.Time) error {
	r.markedFailed = true
	return nil
}

func (r *fakeRepository) CreateNotifyLog(ctx context.Context, input NotifyLogMutation) (int64, error) {
	return 1, nil
}

func (r *fakeRepository) UpdateNotifyLog(ctx context.Context, id int64, input NotifyLogUpdate) error {
	if input.ProcessStatus == 2 {
		r.notifySuccess = true
	}
	if input.ProcessStatus == 3 {
		r.notifyFailed = true
	}
	return nil
}

func (r *fakeRepository) FindTransactionByNoForUpdate(ctx context.Context, transactionNo string) (*PayTransaction, error) {
	return r.notifyTxn, nil
}

func (r *fakeRepository) MarkPaySuccessAndCreditRecharge(ctx context.Context, input PaySuccessMutation) (*PaySuccessResult, error) {
	r.markedPaySuccess = true
	r.creditCalls++
	if r.paySuccessResult != nil {
		return r.paySuccessResult, nil
	}
	return &PaySuccessResult{}, nil
}

type fakeGateway struct {
	createResp   *payalipay.CreateResponse
	createErr    error
	notifyResult *payalipay.NotifyResult
	verifyErr    error
}

func (g *fakeGateway) Create(ctx context.Context, cfg payalipay.ChannelConfig, req payalipay.CreateRequest) (*payalipay.CreateResponse, error) {
	if g.createErr != nil {
		return nil, g.createErr
	}
	return g.createResp, nil
}

func (g *fakeGateway) VerifyNotify(ctx context.Context, cfg payalipay.ChannelConfig, req payalipay.NotifyRequest) (*payalipay.NotifyResult, error) {
	if g.verifyErr != nil {
		return nil, g.verifyErr
	}
	if g.notifyResult == nil {
		return nil, errors.New("not implemented")
	}
	return g.notifyResult, nil
}

func (g *fakeGateway) SuccessBody() string { return "success" }
func (g *fakeGateway) FailureBody() string { return "fail" }

type fakeSecretbox struct{}

func (fakeSecretbox) Decrypt(ciphertext string) (string, error) { return "private-key", nil }

type fakeCertResolver struct{}

func (fakeCertResolver) Resolve(storedPath string) (string, error) { return storedPath, nil }

type staticNumberGenerator string

func (g staticNumberGenerator) Next(ctx context.Context, prefix string) (string, error) {
	return string(g), nil
}

type noopLocker struct{}

func (noopLocker) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "token", nil
}

func (noopLocker) Unlock(ctx context.Context, key string, token string) error {
	return nil
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
}

func activeAlipayChannel() *Channel {
	return &Channel{
		ID:               1,
		Channel:          2,
		AppID:            "app-id",
		AppPrivateKeyEnc: "cipher",
		PublicCertPath:   "app.crt",
		PlatformCertPath: "alipay.crt",
		RootCertPath:     "root.crt",
		NotifyURL:        "https://example.test/api/pay/notify/alipay",
		IsSandbox:        1,
		Status:           1,
	}
}

func payCreateResponse() *payalipay.CreateResponse {
	return &payalipay.CreateResponse{Mode: "external", Content: "https://openapi.alipaydev.com/pay", Raw: map[string]any{"content": "https://openapi.alipaydev.com/pay"}}
}
