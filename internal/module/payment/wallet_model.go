package payment

import "time"

type Wallet struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	UserID             int64     `gorm:"column:user_id"`
	BalanceCents       int64     `gorm:"column:balance_cents"`
	TotalRechargeCents int64     `gorm:"column:total_recharge_cents"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Wallet) TableName() string { return "user_wallets" }

type WalletTransaction struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	TransactionNo      string    `gorm:"column:transaction_no"`
	WalletID           int64     `gorm:"column:wallet_id"`
	UserID             int64     `gorm:"column:user_id"`
	Direction          string    `gorm:"column:direction"`
	AmountCents        int64     `gorm:"column:amount_cents"`
	BalanceBeforeCents int64     `gorm:"column:balance_before_cents"`
	BalanceAfterCents  int64     `gorm:"column:balance_after_cents"`
	SourceType         string    `gorm:"column:source_type"`
	SourceID           int64     `gorm:"column:source_id"`
	Remark             string    `gorm:"column:remark"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (WalletTransaction) TableName() string { return "wallet_transactions" }
