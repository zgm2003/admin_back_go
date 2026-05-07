package payruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	FindActiveAlipayChannel(ctx context.Context, channelID int64) (*Channel, error)
	FindLatestOngoingRechargeByUser(ctx context.Context, userID int64) (*Order, error)
	CreateRechargeOrder(ctx context.Context, input RechargeOrderMutation) (*RechargeOrderCreated, error)
	ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) ([]CurrentUserOrderRow, int64, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, error)
	GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error)
	FindLastAnyTransactionForOrder(ctx context.Context, orderID int64, successTxnID int64) (*PayTransaction, error)
	FindLastActiveTransactionForUpdate(ctx context.Context, orderID int64) (*PayTransaction, error)
	CloseCurrentUserRechargeOrder(ctx context.Context, orderID int64, currentStatus int, reason string, now time.Time) (int64, error)
	CloseTransaction(ctx context.Context, txnID int64, now time.Time) error
	ListExpiredRechargeOrders(ctx context.Context, cutoff time.Time, limit int) ([]ExpiredRechargeOrder, error)
	ListPendingAlipayTransactions(ctx context.Context, cutoff time.Time, limit int) ([]PendingTransaction, error)
	CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryRow, error)
	CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) ([]WalletBillRow, int64, error)
	CreateTransaction(ctx context.Context, input TransactionMutation) (*PayTransaction, error)
	MarkTransactionWaiting(ctx context.Context, txnID int64, raw map[string]any, now time.Time) error
	MarkTransactionFailed(ctx context.Context, txnID int64, reason string, now time.Time) error
	CreateNotifyLog(ctx context.Context, input NotifyLogMutation) (int64, error)
	UpdateNotifyLog(ctx context.Context, id int64, input NotifyLogUpdate) error
	FindTransactionByNoForUpdate(ctx context.Context, transactionNo string) (*PayTransaction, error)
	MarkPaySuccessAndCreditRecharge(ctx context.Context, input PaySuccessMutation) (*PaySuccessResult, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (r *GormRepository) FindActiveAlipayChannel(ctx context.Context, channelID int64) (*Channel, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Channel
	err := r.db.WithContext(ctx).
		Where("id = ?", channelID).
		Where("channel = ?", enum.PayChannelAlipay).
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) FindLatestOngoingRechargeByUser(ctx context.Context, userID int64) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("order_type = ?", enum.PayOrderRecharge).
		Where("pay_status IN ?", []int{enum.PayStatusPending, enum.PayStatusPaying}).
		Where("is_del = ?", enum.CommonNo).
		Order("id desc").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateRechargeOrder(ctx context.Context, input RechargeOrderMutation) (*RechargeOrderCreated, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var created *RechargeOrderCreated
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order := Order{
			OrderNo:        input.OrderNo,
			UserID:         input.UserID,
			OrderType:      enum.PayOrderRecharge,
			BizType:        "recharge",
			BizID:          0,
			Title:          input.Title,
			ItemCount:      1,
			TotalAmount:    input.Amount,
			DiscountAmount: 0,
			PayAmount:      input.Amount,
			PayStatus:      enum.PayStatusPending,
			BizStatus:      enum.PayBizInit,
			ChannelID:      input.ChannelID,
			PayMethod:      input.PayMethod,
			ExpireTime:     input.ExpireTime,
			IP:             input.IP,
			IsDel:          enum.CommonNo,
		}
		if err := tx.WithContext(ctx).Create(&order).Error; err != nil {
			return err
		}
		item := OrderItem{
			OrderID:  order.ID,
			ItemType: "recharge",
			Title:    input.Title,
			Price:    input.Amount,
			Quantity: 1,
			Amount:   input.Amount,
			IsDel:    enum.CommonNo,
		}
		if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
			return err
		}
		created = &RechargeOrderCreated{OrderID: order.ID, OrderNo: order.OrderNo, PayAmount: order.PayAmount, ExpireTime: order.ExpireTime}
		return nil
	})
	return created, err
}

func (r *GormRepository) ListCurrentUserRechargeOrders(ctx context.Context, userID int64, query CurrentUserOrderListQuery) ([]CurrentUserOrderRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Table("orders AS o").
		Joins("LEFT JOIN pay_channel AS pc ON pc.id = o.channel_id AND pc.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN pay_transactions AS pt ON pt.id = (SELECT ptx.id FROM pay_transactions AS ptx WHERE ptx.order_id = o.id AND ptx.is_del = ? ORDER BY ptx.attempt_no DESC, ptx.id DESC LIMIT 1)", enum.CommonNo).
		Where("o.user_id = ?", userID).
		Where("o.order_type = ?", enum.PayOrderRecharge).
		Where("o.is_del = ?", enum.CommonNo)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []CurrentUserOrderRow
	err := db.Select(`
			o.id,
			o.order_no,
			o.title,
			o.pay_amount,
			o.pay_status,
			o.biz_status,
			o.pay_time,
			o.created_at,
			o.expire_time,
			o.channel_id,
			pc.name AS channel_name,
			o.pay_method,
			pt.transaction_no,
			pt.status AS transaction_status
		`).
		Order("o.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetOrderByNo(ctx context.Context, orderNo string) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).
		Where("order_no = ?", strings.TrimSpace(orderNo)).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Order
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_no = ?", strings.TrimSpace(orderNo)).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) FindLastActiveTransactionForUpdate(ctx context.Context, orderID int64) (*PayTransaction, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row PayTransaction
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", orderID).
		Where("status IN ?", []int{enum.PayTxnCreated, enum.PayTxnWaiting}).
		Where("is_del = ?", enum.CommonNo).
		Order("attempt_no desc, id desc").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) FindLastAnyTransactionForOrder(ctx context.Context, orderID int64, successTxnID int64) (*PayTransaction, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row PayTransaction
	db := r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
	if successTxnID > 0 {
		db = db.Where("id = ?", successTxnID)
	} else {
		db = db.Where("order_id = ?", orderID).Order("attempt_no desc, id desc")
	}
	err := db.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CloseCurrentUserRechargeOrder(ctx context.Context, orderID int64, currentStatus int, reason string, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	result := r.db.WithContext(ctx).
		Model(&Order{}).
		Where("id = ?", orderID).
		Where("order_type = ?", enum.PayOrderRecharge).
		Where("pay_status = ?", currentStatus).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"pay_status": enum.PayStatusClosed, "close_time": now, "close_reason": reason, "updated_at": now})
	return result.RowsAffected, result.Error
}

func (r *GormRepository) CurrentUserWalletSummary(ctx context.Context, userID int64) (*WalletSummaryRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var wallet UserWallet
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		First(&wallet).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &WalletSummaryRow{Exists: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &WalletSummaryRow{Exists: true, Balance: wallet.Balance, Frozen: wallet.Frozen, TotalRecharge: wallet.TotalRecharge, TotalConsume: wallet.TotalConsume, CreatedAt: &wallet.CreatedAt}, nil
}

func (r *GormRepository) CurrentUserWalletBills(ctx context.Context, userID int64, query WalletBillsQuery) ([]WalletBillRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Table("wallet_transactions AS wt").
		Where("wt.user_id = ?", userID).
		Where("wt.is_del = ?", enum.CommonNo)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []WalletBillRow
	err := db.Select(`
			wt.id,
			wt.biz_action_no,
			wt.type,
			wt.available_delta,
			wt.frozen_delta,
			wt.balance_before,
			wt.balance_after,
			wt.title,
			wt.remark,
			wt.order_no,
			wt.created_at
		`).
		Order("wt.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	return rows, total, err
}

func (r *GormRepository) CloseTransaction(ctx context.Context, txnID int64, now time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&PayTransaction{}).
		Where("id = ?", txnID).
		Where("status IN ?", []int{enum.PayTxnCreated, enum.PayTxnWaiting}).
		Where("is_del = ?", enum.CommonNo).
		Updates(map[string]any{"status": enum.PayTxnClosed, "closed_at": now, "updated_at": now}).Error
}

func (r *GormRepository) ListExpiredRechargeOrders(ctx context.Context, cutoff time.Time, limit int) ([]ExpiredRechargeOrder, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultCloseExpiredOrderLimit
	}
	var rows []ExpiredRechargeOrder
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("id, order_no").
		Where("order_type = ?", enum.PayOrderRecharge).
		Where("pay_status IN ?", []int{enum.PayStatusPending, enum.PayStatusPaying}).
		Where("expire_time <= ?", cutoff).
		Where("is_del = ?", enum.CommonNo).
		Order("id asc").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ListPendingAlipayTransactions(ctx context.Context, cutoff time.Time, limit int) ([]PendingTransaction, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = defaultSyncPendingTransactionLimit
	}
	var rows []PendingTransaction
	err := r.db.WithContext(ctx).
		Table("pay_transactions AS pt").
		Joins("JOIN orders AS o ON o.id = pt.order_id AND o.is_del = ?", enum.CommonNo).
		Select(`pt.id, pt.transaction_no, pt.order_id, pt.order_no, pt.channel_id, pt.channel, pt.pay_method, pt.amount, pt.trade_no, pt.status, pt.created_at`).
		Where("pt.channel = ?", enum.PayChannelAlipay).
		Where("pt.status IN ?", []int{enum.PayTxnCreated, enum.PayTxnWaiting}).
		Where("o.pay_status IN ?", []int{enum.PayStatusPending, enum.PayStatusPaying}).
		Where("pt.created_at <= ?", cutoff).
		Where("pt.is_del = ?", enum.CommonNo).
		Order("pt.id asc").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) CreateTransaction(ctx context.Context, input TransactionMutation) (*PayTransaction, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	row := PayTransaction{
		TransactionNo: input.TransactionNo,
		OrderID:       input.OrderID,
		OrderNo:       input.OrderNo,
		AttemptNo:     input.AttemptNo,
		ChannelID:     input.ChannelID,
		Channel:       input.Channel,
		PayMethod:     input.PayMethod,
		Amount:        input.Amount,
		Status:        enum.PayTxnCreated,
		IsDel:         enum.CommonNo,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) MarkTransactionWaiting(ctx context.Context, txnID int64, raw map[string]any, now time.Time) error {
	return r.updateTransactionStatus(ctx, txnID, enum.PayTxnWaiting, map[string]any{"channel_resp": mustJSON(raw), "updated_at": now})
}

func (r *GormRepository) MarkTransactionFailed(ctx context.Context, txnID int64, reason string, now time.Time) error {
	return r.updateTransactionStatus(ctx, txnID, enum.PayTxnFailed, map[string]any{"channel_resp": mustJSON(map[string]any{"error": reason}), "updated_at": now})
}

func (r *GormRepository) updateTransactionStatus(ctx context.Context, txnID int64, status int, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	fields["status"] = status
	return r.db.WithContext(ctx).
		Model(&PayTransaction{}).
		Where("id = ?", txnID).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) CreateNotifyLog(ctx context.Context, input NotifyLogMutation) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	row := PayNotifyLog{
		Channel:        input.Channel,
		NotifyType:     enum.NotifyPay,
		TransactionNo:  input.TransactionNo,
		TradeNo:        input.TradeNo,
		Headers:        mustJSON(input.Headers),
		RawData:        mustJSON(input.RawData),
		ProcessStatus:  enum.NotifyProcessPending,
		ProcessMessage: "待处理",
		IP:             input.IP,
		IsDel:          enum.CommonNo,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) UpdateNotifyLog(ctx context.Context, id int64, input NotifyLogUpdate) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil
	}
	fields := map[string]any{
		"transaction_no": input.TransactionNo,
		"trade_no":       input.TradeNo,
		"process_status": input.ProcessStatus,
		"process_msg":    input.ProcessMessage,
		"updated_at":     input.Now,
	}
	return r.db.WithContext(ctx).Model(&PayNotifyLog{}).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) FindTransactionByNoForUpdate(ctx context.Context, transactionNo string) (*PayTransaction, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row PayTransaction
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("transaction_no = ?", strings.TrimSpace(transactionNo)).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) MarkPaySuccessAndCreditRecharge(ctx context.Context, input PaySuccessMutation) (*PaySuccessResult, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var result *PaySuccessResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res, err := markPaySuccessAndCreditRechargeInTx(ctx, tx, input)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil && isDuplicateKey(err) {
		return &PaySuccessResult{AlreadySuccess: true}, nil
	}
	return result, err
}

func markPaySuccessAndCreditRechargeInTx(ctx context.Context, tx *gorm.DB, input PaySuccessMutation) (*PaySuccessResult, error) {
	var txn PayTransaction
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("transaction_no = ?", input.TransactionNo).
		Where("is_del = ?", enum.CommonNo).
		First(&txn).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	if txn.Status == enum.PayTxnSuccess {
		return &PaySuccessResult{AlreadySuccess: true, TransactionID: txn.ID, OrderID: txn.OrderID, OrderNo: txn.OrderNo}, nil
	}

	var order Order
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", txn.OrderID).
		Where("is_del = ?", enum.CommonNo).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}

	now := input.PaidAt
	if now.IsZero() {
		now = time.Now()
	}
	raw := mustJSON(input.RawNotify)
	if err := tx.WithContext(ctx).Model(&PayTransaction{}).Where("id = ?", txn.ID).Updates(map[string]any{
		"status":       enum.PayTxnSuccess,
		"trade_no":     input.TradeNo,
		"trade_status": input.TradeStatus,
		"paid_at":      now,
		"raw_notify":   raw,
		"updated_at":   now,
	}).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Model(&Order{}).Where("id = ?", order.ID).Updates(map[string]any{
		"pay_status":             enum.PayStatusPaid,
		"biz_status":             enum.PayBizSuccess,
		"pay_time":               now,
		"biz_done_at":            now,
		"success_transaction_id": txn.ID,
		"channel_id":             txn.ChannelID,
		"pay_method":             txn.PayMethod,
		"updated_at":             now,
	}).Error; err != nil {
		return nil, err
	}

	wallet, err := lockOrCreateWallet(ctx, tx, order.UserID)
	if err != nil {
		return nil, err
	}
	bizActionNo := "WALLET:RECHARGE:" + order.OrderNo
	var existing WalletTransaction
	err = tx.WithContext(ctx).Where("biz_action_no = ?", bizActionNo).First(&existing).Error
	if err == nil {
		return &PaySuccessResult{AlreadySuccess: true, OrderID: order.ID, OrderNo: order.OrderNo, TransactionID: txn.ID, WalletBefore: existing.BalanceBefore, WalletAfter: existing.BalanceAfter}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	before := wallet.Balance
	after := before + order.PayAmount
	if err := tx.WithContext(ctx).Model(&UserWallet{}).Where("id = ?", wallet.ID).Updates(map[string]any{
		"balance":        after,
		"total_recharge": gorm.Expr("total_recharge + ?", order.PayAmount),
		"version":        gorm.Expr("version + 1"),
		"updated_at":     now,
	}).Error; err != nil {
		return nil, err
	}
	fulfillment, err := createRechargeFulfillment(ctx, tx, order, txn, input.FulfillNo, now)
	if err != nil {
		return nil, err
	}
	walletTxn := WalletTransaction{
		BizActionNo:    bizActionNo,
		UserID:         order.UserID,
		WalletID:       wallet.ID,
		Type:           enum.WalletTypeRecharge,
		AvailableDelta: order.PayAmount,
		BalanceBefore:  before,
		BalanceAfter:   after,
		FrozenBefore:   wallet.Frozen,
		FrozenAfter:    wallet.Frozen,
		OrderID:        order.ID,
		OrderNo:        order.OrderNo,
		SourceType:     enum.WalletSourceFulfill,
		SourceID:       fulfillment.ID,
		Title:          "充值入账",
		Remark:         "充值入账",
		Ext:            mustJSON(map[string]any{"amount": order.PayAmount, "transaction_no": txn.TransactionNo}),
		IsDel:          enum.CommonNo,
	}
	if err := tx.WithContext(ctx).Create(&walletTxn).Error; err != nil {
		return nil, err
	}
	return &PaySuccessResult{OrderID: order.ID, OrderNo: order.OrderNo, TransactionID: txn.ID, WalletBefore: before, WalletAfter: after}, nil
}

func lockOrCreateWallet(ctx context.Context, tx *gorm.DB, userID int64) (*UserWallet, error) {
	var wallet UserWallet
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Where("is_del = ?", enum.CommonNo).
		First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	wallet = UserWallet{UserID: userID, IsDel: enum.CommonNo}
	if err := tx.WithContext(ctx).Create(&wallet).Error; err != nil {
		return nil, err
	}
	return &wallet, nil
}

func createRechargeFulfillment(ctx context.Context, tx *gorm.DB, order Order, txn PayTransaction, fulfillNo string, now time.Time) (*OrderFulfillment, error) {
	if strings.TrimSpace(fulfillNo) == "" {
		fulfillNo = "D" + order.OrderNo
	}
	fulfillment := OrderFulfillment{
		FulfillNo:      fulfillNo,
		OrderID:        order.ID,
		OrderNo:        order.OrderNo,
		UserID:         order.UserID,
		BizType:        order.BizType,
		BizID:          order.BizID,
		ActionType:     enum.FulfillActionRecharge,
		SourceTxnID:    txn.ID,
		IdempotencyKey: "FULFILL:RECHARGE:" + order.OrderNo,
		Status:         enum.FulfillSuccess,
		ExecutedAt:     &now,
		RequestPayload: mustJSON(map[string]any{"order_id": order.ID, "order_no": order.OrderNo, "user_id": order.UserID, "amount": order.PayAmount, "biz_type": order.BizType, "biz_id": order.BizID}),
		ResultPayload:  mustJSON(map[string]any{"wallet_credited": true}),
		IsDel:          enum.CommonNo,
	}
	err := tx.WithContext(ctx).Create(&fulfillment).Error
	if err != nil && isDuplicateKey(err) {
		var existing OrderFulfillment
		findErr := tx.WithContext(ctx).Where("idempotency_key = ?", fulfillment.IdempotencyKey).First(&existing).Error
		if findErr != nil {
			return nil, findErr
		}
		return &existing, nil
	}
	return &fulfillment, err
}

func mustJSON(value any) string {
	buf, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
