package payruntime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/payment"
	payalipay "admin_back_go/internal/platform/payment/alipay"
	"admin_back_go/internal/platform/redislock"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"
const (
	defaultCloseExpiredOrderLimit      = 50
	defaultSyncPendingTransactionLimit = 100
	rechargeOrderExpireDuration        = 30 * time.Minute
	pendingTransactionSyncDelay        = 5 * time.Minute
)

type secretDecrypter interface {
	Decrypt(ciphertext string) (string, error)
}

type certResolver interface {
	Resolve(storedPath string) (string, error)
}

type Dependencies struct {
	Repository      Repository
	Gateway         payalipay.Gateway
	Secretbox       secretDecrypter
	CertResolver    certResolver
	Locker          redislock.Locker
	NumberGenerator NumberGenerator
	Now             func() time.Time
	NotifyLockTTL   time.Duration
	AttemptLockTTL  time.Duration
}

type Service struct {
	repository      Repository
	gateway         payalipay.Gateway
	secretbox       secretDecrypter
	certResolver    certResolver
	locker          redislock.Locker
	numberGenerator NumberGenerator
	now             func() time.Time
	notifyLockTTL   time.Duration
	attemptLockTTL  time.Duration
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	notifyTTL := deps.NotifyLockTTL
	if notifyTTL <= 0 {
		notifyTTL = 30 * time.Second
	}
	attemptTTL := deps.AttemptLockTTL
	if attemptTTL <= 0 {
		attemptTTL = 30 * time.Second
	}
	gateway := deps.Gateway
	if gateway == nil {
		gateway = payalipay.NewGopayGateway(0)
	}
	box := deps.Secretbox
	if box == nil {
		defaultBox := secretbox.New("")
		box = defaultBox
	}
	resolver := deps.CertResolver
	if resolver == nil {
		resolver = payment.CertPathResolver{WorkingDir: "."}
	}
	return &Service{
		repository:      deps.Repository,
		gateway:         gateway,
		secretbox:       box,
		certResolver:    resolver,
		locker:          deps.Locker,
		numberGenerator: deps.NumberGenerator,
		now:             now,
		notifyLockTTL:   notifyTTL,
		attemptLockTTL:  attemptTTL,
	}
}

func (s *Service) CreateRechargeOrder(ctx context.Context, userID int64, input RechargeOrderCreateInput) (*RechargeOrderCreateResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	if !isRechargePreset(input.Amount) {
		return nil, apperror.BadRequest("不支持的充值金额")
	}
	if !isAlipayRuntimeMethod(input.PayMethod) {
		return nil, apperror.BadRequest("不支持的支付方式")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	channel, err := repo.FindActiveAlipayChannel(ctx, input.ChannelID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	if channel == nil {
		return nil, apperror.BadRequest("支付渠道不可用")
	}
	if !isChannelMethodSupported(channel, input.PayMethod) {
		return nil, apperror.BadRequest("该渠道未配置当前支付方式")
	}
	ongoing, err := repo.FindLatestOngoingRechargeByUser(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询充值订单失败", err)
	}
	if ongoing != nil && ongoing.ExpireTime.After(s.now()) {
		return nil, apperror.BadRequest("请先完成或取消当前未支付的充值订单")
	}
	orderNo, appErr := s.nextNo(ctx, "R")
	if appErr != nil {
		return nil, appErr
	}
	created, err := repo.CreateRechargeOrder(ctx, RechargeOrderMutation{
		OrderNo:    orderNo,
		UserID:     userID,
		Amount:     input.Amount,
		PayMethod:  strings.TrimSpace(input.PayMethod),
		ChannelID:  input.ChannelID,
		Title:      buildRechargeTitle(input.Amount),
		ExpireTime: s.now().Add(30 * time.Minute),
		IP:         strings.TrimSpace(input.IP),
		Now:        s.now(),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建充值订单失败", err)
	}
	return &RechargeOrderCreateResponse{
		OrderID: created.OrderID, OrderNo: created.OrderNo, PayAmount: created.PayAmount, ExpireTime: formatTime(created.ExpireTime),
	}, nil
}

func (s *Service) CreatePayAttempt(ctx context.Context, userID int64, orderNo string, input PayAttemptCreateInput) (*PayAttemptCreateResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, apperror.BadRequest("订单号不能为空")
	}
	if token, ok, appErr := s.lock(ctx, "pay_create_txn_"+orderNo, s.attemptLockTTL); appErr != nil || ok {
		if appErr != nil {
			return nil, appErr
		}
		defer s.unlock(ctx, "pay_create_txn_"+orderNo, token)
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	var order *Order
	var channel *Channel
	var txn *PayTransaction
	err := repo.WithTx(ctx, func(txRepo Repository) error {
		var txErr error
		order, txErr = txRepo.GetOrderByNoForUpdate(ctx, orderNo)
		if txErr != nil {
			return txErr
		}
		if order == nil {
			return ErrOrderNotFound
		}
		if order.UserID != userID {
			return apperror.Forbidden("无权操作该订单")
		}
		if order.PayStatus != enum.PayStatusPending && order.PayStatus != enum.PayStatusPaying {
			return ErrOrderConflict
		}
		payMethod := choosePayMethod(input.PayMethod, order.PayMethod)
		if !isAlipayRuntimeMethod(payMethod) {
			return ErrOrderConflict
		}
		channel, txErr = txRepo.FindActiveAlipayChannel(ctx, order.ChannelID)
		if txErr != nil {
			return txErr
		}
		if channel == nil || !isChannelMethodSupported(channel, payMethod) {
			return ErrOrderConflict
		}
		lastTxn, txErr := txRepo.FindLastActiveTransactionForUpdate(ctx, order.ID)
		if txErr != nil {
			return txErr
		}
		attemptNo := 1
		if lastTxn != nil {
			attemptNo = lastTxn.AttemptNo + 1
			if err := txRepo.CloseTransaction(ctx, lastTxn.ID, s.now()); err != nil {
				return err
			}
		}
		transactionNo, appErr := s.nextNo(ctx, "T")
		if appErr != nil {
			return appErr
		}
		txn, txErr = txRepo.CreateTransaction(ctx, TransactionMutation{
			TransactionNo: transactionNo,
			OrderID:       order.ID,
			OrderNo:       order.OrderNo,
			AttemptNo:     attemptNo,
			ChannelID:     channel.ID,
			Channel:       channel.Channel,
			PayMethod:     payMethod,
			Amount:        order.PayAmount,
			Now:           s.now(),
		})
		return txErr
	})
	if appErr := mapAttemptCreateError(err); appErr != nil {
		return nil, appErr
	}
	cfg, appErr := s.alipayConfig(channel)
	if appErr != nil {
		_ = repo.MarkTransactionFailed(ctx, txn.ID, appErr.Message, s.now())
		return nil, appErr
	}
	resp, gatewayErr := s.gateway.Create(ctx, cfg, payalipay.CreateRequest{
		OutTradeNo:  txn.TransactionNo,
		Subject:     truncateSubject(order.Title),
		AmountCents: order.PayAmount,
		PayMethod:   txn.PayMethod,
		ReturnURL:   strings.TrimSpace(input.ReturnURL),
	})
	if gatewayErr != nil {
		_ = repo.MarkTransactionFailed(ctx, txn.ID, gatewayErr.Error(), s.now())
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建支付宝支付请求失败", gatewayErr)
	}
	if err := repo.MarkTransactionWaiting(ctx, txn.ID, resp.Raw, s.now()); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "更新支付流水失败", err)
	}
	return &PayAttemptCreateResponse{
		TransactionNo: txn.TransactionNo,
		TransactionID: txn.ID,
		OrderNo:       order.OrderNo,
		PayAmount:     order.PayAmount,
		Channel:       channel.Channel,
		PayMethod:     txn.PayMethod,
		NotifyURL:     channel.NotifyURL,
		ReturnURL:     strings.TrimSpace(input.ReturnURL),
		PayData:       map[string]any{"mode": resp.Mode, "content": resp.Content},
	}, nil
}

func (s *Service) ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) (*CurrentUserOrderListResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeCurrentUserOrderListQuery(query)
	rows, total, err := repo.ListCurrentUserRechargeOrders(ctx, userID, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询充值订单失败", err)
	}
	list := make([]CurrentUserOrderItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, currentUserOrderItem(row))
	}
	return &CurrentUserOrderListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) QueryCurrentUserRechargeResult(ctx context.Context, userID int64, orderNo string) (*OrderQueryResultResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, apperror.BadRequest("订单号不能为空")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	order, err := repo.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询订单失败", err)
	}
	if order == nil || order.OrderType != enum.PayOrderRecharge {
		return nil, apperror.NotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, apperror.Forbidden("无权查看该订单")
	}
	txn, err := repo.FindLastAnyTransactionForOrder(ctx, order.ID, order.SuccessTransactionID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付流水失败", err)
	}
	return orderQueryResult(order, txn), nil
}

func (s *Service) CancelCurrentUserRechargeOrder(ctx context.Context, userID int64, orderNo string, input CancelOrderInput) *apperror.Error {
	if userID <= 0 {
		return apperror.Unauthorized("未登录")
	}
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return apperror.BadRequest("订单号不能为空")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "用户取消订单"
	}
	if len([]rune(reason)) > 100 {
		return apperror.BadRequest("取消原因不能超过100个字符")
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	var cancelErr *apperror.Error
	err := repo.WithTx(ctx, func(tx Repository) error {
		order, err := tx.GetOrderByNoForUpdate(ctx, orderNo)
		if err != nil {
			return err
		}
		if order == nil || order.OrderType != enum.PayOrderRecharge {
			cancelErr = apperror.NotFound("订单不存在")
			return nil
		}
		if order.UserID != userID {
			cancelErr = apperror.Forbidden("无权操作该订单")
			return nil
		}
		if order.PayStatus != enum.PayStatusPending && order.PayStatus != enum.PayStatusPaying {
			cancelErr = apperror.BadRequest("该订单状态不允许取消")
			return nil
		}
		affected, err := tx.CloseCurrentUserRechargeOrder(ctx, order.ID, order.PayStatus, reason, now)
		if err != nil {
			return err
		}
		if affected == 0 {
			cancelErr = apperror.BadRequest("订单状态已变更，请刷新后重试")
			return nil
		}
		lastTxn, err := tx.FindLastActiveTransactionForUpdate(ctx, order.ID)
		if err != nil {
			return err
		}
		if lastTxn != nil {
			return tx.CloseTransaction(ctx, lastTxn.ID, now)
		}
		return nil
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "取消充值订单失败", err)
	}
	return cancelErr
}

func (s *Service) CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.CurrentUserWalletSummary(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询钱包失败", err)
	}
	if row == nil || !row.Exists {
		return &WalletSummaryResponse{WalletExists: enum.CommonNo}, nil
	}
	createdAt := ""
	if row.CreatedAt != nil {
		createdAt = formatTime(*row.CreatedAt)
	}
	return &WalletSummaryResponse{
		WalletExists: enum.CommonYes, Balance: row.Balance, Frozen: row.Frozen,
		TotalRecharge: row.TotalRecharge, TotalConsume: row.TotalConsume, CreatedAt: createdAt,
	}, nil
}

func (s *Service) CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) (*WalletBillsResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("未登录")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeWalletBillsQuery(query)
	rows, total, err := repo.CurrentUserWalletBills(ctx, userID, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询钱包流水失败", err)
	}
	list := make([]WalletBillItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, walletBillItem(row))
	}
	return &WalletBillsResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) HandleAlipayNotify(ctx context.Context, input AlipayNotifyInput) (string, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return s.gateway.FailureBody(), appErr
	}
	now := s.now()
	logID, _ := repo.CreateNotifyLog(ctx, NotifyLogMutation{
		Channel:       enum.PayChannelAlipay,
		TransactionNo: input.Form["out_trade_no"],
		TradeNo:       input.Form["trade_no"],
		Headers:       input.Headers,
		RawData:       input.Form,
		IP:            strings.TrimSpace(input.IP),
		Now:           now,
	})
	fail := func(message string, cause error) (string, *apperror.Error) {
		_ = repo.UpdateNotifyLog(ctx, logID, NotifyLogUpdate{
			TransactionNo:  input.Form["out_trade_no"],
			TradeNo:        input.Form["trade_no"],
			ProcessStatus:  enum.NotifyProcessFailed,
			ProcessMessage: message,
			Now:            s.now(),
		})
		if cause != nil {
			return s.gateway.FailureBody(), apperror.Wrap(apperror.CodeBadRequest, 400, message, cause)
		}
		return s.gateway.FailureBody(), apperror.BadRequest(message)
	}

	outTradeNo := strings.TrimSpace(input.Form["out_trade_no"])
	if outTradeNo == "" {
		return fail("回调缺少交易号", nil)
	}
	if token, ok, appErr := s.lock(ctx, "pay_notify_"+outTradeNo, s.notifyLockTTL); appErr != nil || ok {
		if appErr != nil {
			return fail(appErr.Message, appErr)
		}
		defer s.unlock(ctx, "pay_notify_"+outTradeNo, token)
	}

	txn, err := repo.FindTransactionByNoForUpdate(ctx, outTradeNo)
	if err != nil {
		return fail("查询支付流水失败", err)
	}
	if txn == nil {
		return fail("支付流水不存在", ErrTransactionNotFound)
	}
	channel, err := repo.FindActiveAlipayChannel(ctx, txn.ChannelID)
	if err != nil {
		return fail("查询支付渠道失败", err)
	}
	if channel == nil {
		return fail("支付渠道不可用", nil)
	}
	cfg, appErr := s.alipayConfig(channel)
	if appErr != nil {
		return fail(appErr.Message, appErr)
	}
	result, err := s.gateway.VerifyNotify(ctx, cfg, payalipay.NotifyRequest{Form: input.Form})
	if err != nil {
		return fail("支付宝回调验签失败", err)
	}
	if err := validateAlipayNotifyResult(channel, txn, result); err != nil {
		return fail(err.Error(), err)
	}
	fulfillNo, appErr := s.nextNo(ctx, "D")
	if appErr != nil {
		return fail(appErr.Message, appErr)
	}
	success, err := repo.MarkPaySuccessAndCreditRecharge(ctx, PaySuccessMutation{
		TransactionNo: result.OutTradeNo,
		TradeNo:       result.TradeNo,
		TradeStatus:   result.TradeStatus,
		RawNotify:     result.Raw,
		PaidAt:        now,
		FulfillNo:     fulfillNo,
	})
	if err != nil {
		return fail("处理支付成功失败", err)
	}
	message := "支付成功"
	if success != nil && success.AlreadySuccess {
		message = "交易已成功"
	}
	_ = repo.UpdateNotifyLog(ctx, logID, NotifyLogUpdate{
		TransactionNo:  result.OutTradeNo,
		TradeNo:        result.TradeNo,
		ProcessStatus:  enum.NotifyProcessSuccess,
		ProcessMessage: message,
		Now:            s.now(),
	})
	return s.gateway.SuccessBody(), nil
}

func (s *Service) CloseExpiredOrders(ctx context.Context, input CloseExpiredOrderInput) (*CloseExpiredOrderResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultCloseExpiredOrderLimit
	}
	rows, err := repo.ListExpiredRechargeOrders(ctx, now.Add(-rechargeOrderExpireDuration), limit)
	if err != nil {
		return nil, fmt.Errorf("list expired recharge orders: %w", err)
	}
	result := &CloseExpiredOrderResult{Scanned: len(rows)}
	for _, row := range rows {
		switch s.syncPaidOrCloseOrder(ctx, strings.TrimSpace(row.OrderNo), "支付超时自动关闭", "cron_repair", now) {
		case "paid":
			result.Paid++
		case "closed":
			result.Closed++
		case "deferred":
			result.Deferred++
		default:
			result.Skipped++
		}
	}
	return result, nil
}

func (s *Service) SyncPendingTransactions(ctx context.Context, input SyncPendingTransactionInput) (*SyncPendingTransactionResult, error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultSyncPendingTransactionLimit
	}
	rows, err := repo.ListPendingAlipayTransactions(ctx, now.Add(-pendingTransactionSyncDelay), limit)
	if err != nil {
		return nil, fmt.Errorf("list pending alipay transactions: %w", err)
	}
	result := &SyncPendingTransactionResult{Scanned: len(rows)}
	for _, row := range rows {
		status, err := s.queryTransactionAndMarkPaid(ctx, transactionFromPending(row), "cron_sync", now)
		if err != nil {
			result.Deferred++
			continue
		}
		switch status {
		case "paid":
			result.Paid++
		case "unpaid":
			result.Unpaid++
		default:
			result.Skipped++
		}
	}
	return result, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付运行时仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) syncPaidOrCloseOrder(ctx context.Context, orderNo string, closeReason string, source string, now time.Time) string {
	if orderNo == "" {
		return "skipped"
	}
	if token, ok, appErr := s.lock(ctx, "pay_create_txn_"+orderNo, s.attemptLockTTL); appErr != nil || ok {
		if appErr != nil {
			return "deferred"
		}
		defer s.unlock(ctx, "pay_create_txn_"+orderNo, token)
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return "deferred"
	}

	var order *Order
	var txn *PayTransaction
	err := repo.WithTx(ctx, func(tx Repository) error {
		var txErr error
		order, txErr = tx.GetOrderByNoForUpdate(ctx, orderNo)
		if txErr != nil {
			return txErr
		}
		if order == nil || order.OrderType != enum.PayOrderRecharge {
			return ErrOrderNotFound
		}
		if order.PayStatus != enum.PayStatusPending && order.PayStatus != enum.PayStatusPaying {
			return ErrOrderConflict
		}
		txn, txErr = tx.FindLastActiveTransactionForUpdate(ctx, order.ID)
		return txErr
	})
	if err != nil {
		return "skipped"
	}
	if txn == nil {
		return s.closeOrderAndTransaction(ctx, orderNo, closeReason, now)
	}
	if txn.Channel != enum.PayChannelAlipay {
		return "skipped"
	}

	status, queryErr := s.queryTransactionAndMarkPaid(ctx, txn, source, now)
	if queryErr != nil {
		return "deferred"
	}
	if status == "paid" {
		return "paid"
	}
	if s.closeOrderAndTransaction(ctx, orderNo, closeReason, now) != "closed" {
		return "skipped"
	}
	s.closeAlipayTransactionBestEffort(ctx, txn)
	return "closed"
}

func (s *Service) queryTransactionAndMarkPaid(ctx context.Context, txn *PayTransaction, source string, now time.Time) (string, error) {
	if txn == nil || txn.Channel != enum.PayChannelAlipay {
		return "skipped", nil
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return "skipped", appErr
	}
	channel, err := repo.FindActiveAlipayChannel(ctx, txn.ChannelID)
	if err != nil {
		return "skipped", err
	}
	if channel == nil {
		return "skipped", nil
	}
	cfg, appErr := s.alipayConfig(channel)
	if appErr != nil {
		return "skipped", appErr
	}
	query, err := s.gateway.Query(ctx, cfg, payalipay.QueryRequest{OutTradeNo: txn.TransactionNo, TradeNo: txn.TradeNo})
	if err != nil {
		return "deferred", err
	}
	if !isAlipayPaidStatus(query.TradeStatus) {
		return "unpaid", nil
	}
	if err := validateAlipayQueryResult(channel, txn, query); err != nil {
		return "deferred", err
	}
	fulfillNo, appErr := s.nextNo(ctx, "D")
	if appErr != nil {
		return "deferred", appErr
	}
	_, err = repo.MarkPaySuccessAndCreditRecharge(ctx, PaySuccessMutation{
		TransactionNo: query.OutTradeNo,
		TradeNo:       query.TradeNo,
		TradeStatus:   query.TradeStatus,
		RawNotify:     mergePayQueryRaw(query.Raw, source),
		PaidAt:        paidAtOrNow(query.Raw, now),
		FulfillNo:     fulfillNo,
	})
	if err != nil {
		return "deferred", err
	}
	return "paid", nil
}

func (s *Service) closeOrderAndTransaction(ctx context.Context, orderNo string, reason string, now time.Time) string {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return "deferred"
	}
	err := repo.WithTx(ctx, func(tx Repository) error {
		order, err := tx.GetOrderByNoForUpdate(ctx, orderNo)
		if err != nil {
			return err
		}
		if order == nil || order.OrderType != enum.PayOrderRecharge {
			return ErrOrderNotFound
		}
		if order.PayStatus != enum.PayStatusPending && order.PayStatus != enum.PayStatusPaying {
			return ErrOrderConflict
		}
		affected, err := tx.CloseCurrentUserRechargeOrder(ctx, order.ID, order.PayStatus, reason, now)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrOrderConflict
		}
		lastTxn, err := tx.FindLastActiveTransactionForUpdate(ctx, order.ID)
		if err != nil {
			return err
		}
		if lastTxn != nil {
			return tx.CloseTransaction(ctx, lastTxn.ID, now)
		}
		return nil
	})
	if err != nil {
		return "skipped"
	}
	return "closed"
}

func (s *Service) closeAlipayTransactionBestEffort(ctx context.Context, txn *PayTransaction) {
	if txn == nil || txn.Channel != enum.PayChannelAlipay {
		return
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return
	}
	channel, err := repo.FindActiveAlipayChannel(ctx, txn.ChannelID)
	if err != nil || channel == nil {
		return
	}
	cfg, appErr := s.alipayConfig(channel)
	if appErr != nil {
		return
	}
	_ = s.gateway.Close(ctx, cfg, payalipay.CloseRequest{OutTradeNo: txn.TransactionNo, TradeNo: txn.TradeNo})
}

func (s *Service) nextNo(ctx context.Context, prefix string) (string, *apperror.Error) {
	if s == nil || s.numberGenerator == nil {
		return "", apperror.Internal("支付单号生成器未配置")
	}
	value, err := s.numberGenerator.Next(ctx, prefix)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, 500, "生成支付单号失败", err)
	}
	return value, nil
}

func (s *Service) alipayConfig(channel *Channel) (payalipay.ChannelConfig, *apperror.Error) {
	if channel == nil {
		return payalipay.ChannelConfig{}, apperror.BadRequest("支付渠道不可用")
	}
	privateKey, err := s.secretbox.Decrypt(channel.AppPrivateKeyEnc)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解密支付渠道私钥失败", err)
	}
	appCert, err := s.certResolver.Resolve(channel.PublicCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝应用证书失败", err)
	}
	alipayCert, err := s.certResolver.Resolve(channel.PlatformCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝平台证书失败", err)
	}
	rootCert, err := s.certResolver.Resolve(channel.RootCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝根证书失败", err)
	}
	return payalipay.ChannelConfig{
		ChannelID:      channel.ID,
		AppID:          channel.AppID,
		PrivateKey:     privateKey,
		AppCertPath:    appCert,
		AlipayCertPath: alipayCert,
		RootCertPath:   rootCert,
		NotifyURL:      channel.NotifyURL,
		IsSandbox:      channel.IsSandbox == enum.CommonYes,
	}, nil
}

func (s *Service) lock(ctx context.Context, key string, ttl time.Duration) (string, bool, *apperror.Error) {
	if s == nil || s.locker == nil {
		return "", false, nil
	}
	token, err := s.locker.Lock(ctx, key, ttl)
	if err != nil {
		if errors.Is(err, redislock.ErrNotAcquired) {
			return "", false, apperror.BadRequest("正在处理中，请稍后")
		}
		return "", false, apperror.Wrap(apperror.CodeInternal, 500, "获取支付锁失败", err)
	}
	return token, true, nil
}

func (s *Service) unlock(ctx context.Context, key string, token string) {
	if s == nil || s.locker == nil || token == "" {
		return
	}
	_ = s.locker.Unlock(ctx, key, token)
}

func isRechargePreset(amount int) bool {
	for _, preset := range enum.RechargePresets {
		if preset.Value == amount {
			return true
		}
	}
	return false
}

func isAlipayRuntimeMethod(method string) bool {
	method = strings.TrimSpace(method)
	return method == enum.PayMethodWeb || method == enum.PayMethodH5
}

func isChannelMethodSupported(channel *Channel, method string) bool {
	if channel == nil || channel.Channel != enum.PayChannelAlipay {
		return false
	}
	methods := enum.PayDefaultSupportedMethods(channel.Channel)
	for _, item := range methods {
		if item == strings.TrimSpace(method) {
			return true
		}
	}
	return false
}

func choosePayMethod(input string, fallback string) string {
	input = strings.TrimSpace(input)
	if input != "" {
		return input
	}
	return strings.TrimSpace(fallback)
}

func buildRechargeTitle(amount int) string {
	return fmt.Sprintf("钱包充值 %g 元", float64(amount)/100)
}

func truncateSubject(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "订单支付"
	}
	runes := []rune(title)
	if len(runes) > 128 {
		return string(runes[:128])
	}
	return title
}

func normalizeCurrentUserOrderListQuery(query CurrentUserOrderListQuery) CurrentUserOrderListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 10
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	return query
}

func normalizeWalletBillsQuery(query WalletBillsQuery) WalletBillsQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	return query
}

func currentUserOrderItem(row CurrentUserOrderRow) CurrentUserOrderItem {
	return CurrentUserOrderItem{
		ID: row.ID, OrderNo: row.OrderNo, Title: row.Title, PayAmount: row.PayAmount,
		PayStatus: row.PayStatus, PayStatusText: enum.PayStatusLabels[row.PayStatus],
		BizStatus: row.BizStatus, BizStatusText: enum.PayBizStatusLabels[row.BizStatus],
		PayTime: formatOptionalTime(row.PayTime), CreatedAt: formatTime(row.CreatedAt), ExpireTime: formatOptionalTime(row.ExpireTime),
		ChannelID: row.ChannelID, ChannelName: row.ChannelName, PayMethod: row.PayMethod,
		PayMethodText: enum.PayMethodLabels[row.PayMethod], TransactionNo: row.TransactionNo, TransactionStatus: row.TransactionStatus,
	}
}

func orderQueryResult(order *Order, txn *PayTransaction) *OrderQueryResultResponse {
	result := &OrderQueryResultResponse{
		OrderNo: order.OrderNo, PayStatus: order.PayStatus, BizStatus: order.BizStatus, PayTime: formatOptionalTime(order.PayTime),
	}
	if txn != nil {
		result.Transaction = &TransactionSummary{TransactionNo: txn.TransactionNo, Status: txn.Status, TradeNo: txn.TradeNo}
	}
	return result
}

func walletBillItem(row WalletBillRow) WalletBillItem {
	return WalletBillItem{
		ID: row.ID, BizActionNo: row.BizActionNo, Type: row.Type, TypeText: enum.WalletTypeLabels[row.Type],
		AvailableDelta: row.AvailableDelta, FrozenDelta: row.FrozenDelta, BalanceBefore: row.BalanceBefore, BalanceAfter: row.BalanceAfter,
		Title: row.Title, Remark: row.Remark, OrderNo: row.OrderNo, CreatedAt: formatTime(row.CreatedAt),
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.Format(timeLayout)
	return &text
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func mapAttemptCreateError(err error) *apperror.Error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return apperror.NotFound("订单不存在")
	case errors.Is(err, ErrOrderConflict):
		return apperror.BadRequest("订单状态或支付方式不允许发起支付")
	default:
		return apperror.Wrap(apperror.CodeInternal, 500, "创建支付尝试失败", err)
	}
}

func validateAlipayNotifyResult(channel *Channel, txn *PayTransaction, result *payalipay.NotifyResult) error {
	if result == nil {
		return errors.New("支付宝回调结果为空")
	}
	if strings.TrimSpace(result.OutTradeNo) == "" || result.OutTradeNo != txn.TransactionNo {
		return errors.New("支付宝回调交易号不匹配")
	}
	if strings.TrimSpace(result.AppID) != strings.TrimSpace(channel.AppID) {
		return errors.New("支付宝回调应用ID不匹配")
	}
	if result.TotalAmountCents != txn.Amount {
		return errors.New("支付宝回调金额不匹配")
	}
	if result.TradeStatus != "TRADE_SUCCESS" && result.TradeStatus != "TRADE_FINISHED" {
		return errors.New("支付宝回调交易状态未成功")
	}
	return nil
}

func validateAlipayQueryResult(channel *Channel, txn *PayTransaction, result *payalipay.QueryResult) error {
	if result == nil {
		return errors.New("支付宝查单结果为空")
	}
	if strings.TrimSpace(result.OutTradeNo) == "" || result.OutTradeNo != txn.TransactionNo {
		return errors.New("支付宝查单交易号不匹配")
	}
	if strings.TrimSpace(result.AppID) != "" && strings.TrimSpace(result.AppID) != strings.TrimSpace(channel.AppID) {
		return errors.New("支付宝查单应用ID不匹配")
	}
	if result.TotalAmountCents > 0 && result.TotalAmountCents != txn.Amount {
		return errors.New("支付宝查单金额不匹配")
	}
	return nil
}

func isAlipayPaidStatus(status string) bool {
	status = strings.TrimSpace(status)
	return status == "TRADE_SUCCESS" || status == "TRADE_FINISHED"
}

func transactionFromPending(row PendingTransaction) *PayTransaction {
	return &PayTransaction{
		ID:            row.ID,
		TransactionNo: row.TransactionNo,
		OrderID:       row.OrderID,
		OrderNo:       row.OrderNo,
		ChannelID:     row.ChannelID,
		Channel:       row.Channel,
		PayMethod:     row.PayMethod,
		Amount:        row.Amount,
		TradeNo:       row.TradeNo,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
	}
}

func mergePayQueryRaw(raw map[string]any, source string) map[string]any {
	out := make(map[string]any, len(raw)+1)
	for k, v := range raw {
		out[k] = v
	}
	out["source"] = source
	return out
}

func paidAtOrNow(raw map[string]any, now time.Time) time.Time {
	for _, key := range []string{"gmt_payment", "send_pay_date", "paid_time"} {
		value, ok := raw[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := time.ParseInLocation(timeLayout, strings.TrimSpace(value), time.Local)
		if err == nil {
			return parsed
		}
	}
	return now
}
