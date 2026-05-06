package wallet

import "time"

type UserWallet struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	UserID        int64     `gorm:"column:user_id"`
	Balance       int       `gorm:"column:balance"`
	Frozen        int       `gorm:"column:frozen"`
	TotalRecharge int       `gorm:"column:total_recharge"`
	TotalConsume  int       `gorm:"column:total_consume"`
	Version       int       `gorm:"column:version"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (UserWallet) TableName() string {
	return "user_wallets"
}

type WalletTransaction struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	BizActionNo    string    `gorm:"column:biz_action_no"`
	UserID         int64     `gorm:"column:user_id"`
	WalletID       int64     `gorm:"column:wallet_id"`
	Type           int       `gorm:"column:type"`
	AvailableDelta int       `gorm:"column:available_delta"`
	FrozenDelta    int       `gorm:"column:frozen_delta"`
	BalanceBefore  int       `gorm:"column:balance_before"`
	BalanceAfter   int       `gorm:"column:balance_after"`
	FrozenBefore   int       `gorm:"column:frozen_before"`
	FrozenAfter    int       `gorm:"column:frozen_after"`
	OrderID        int64     `gorm:"column:order_id"`
	OrderNo        string    `gorm:"column:order_no"`
	SourceType     int       `gorm:"column:source_type"`
	SourceID       int64     `gorm:"column:source_id"`
	Title          string    `gorm:"column:title"`
	Remark         string    `gorm:"column:remark"`
	OperatorID     int64     `gorm:"column:operator_id"`
	Ext            string    `gorm:"column:ext"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (WalletTransaction) TableName() string {
	return "wallet_transactions"
}
