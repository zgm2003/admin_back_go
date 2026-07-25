package wallet

import "time"

// Wallet maps user_wallets and owns balance facts for wallet APIs.
type Wallet struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	UserID             int64     `gorm:"column:user_id"`
	BalanceCents       int64     `gorm:"column:balance_cents"`
	TotalRechargeCents int64     `gorm:"column:total_recharge_cents"`
	TotalConsumeCents  int64     `gorm:"column:total_consume_cents"`
	BalanceUnits       int64     `gorm:"column:balance_units"`
	TotalRechargeUnits int64     `gorm:"column:total_recharge_units"`
	TotalConsumeUnits  int64     `gorm:"column:total_consume_units"`
	HeldUnits          int64     `gorm:"column:held_units"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Wallet) TableName() string { return "user_wallets" }

// Transaction maps wallet_transactions. AmountCents is always positive;
// Direction tells whether the money moves in or out.
type Transaction struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	TransactionNo      string    `gorm:"column:transaction_no"`
	WalletID           int64     `gorm:"column:wallet_id"`
	UserID             int64     `gorm:"column:user_id"`
	Direction          string    `gorm:"column:direction"`
	AmountCents        int64     `gorm:"column:amount_cents"`
	BalanceBeforeCents int64     `gorm:"column:balance_before_cents"`
	BalanceAfterCents  int64     `gorm:"column:balance_after_cents"`
	AmountUnits        int64     `gorm:"column:amount_units"`
	BalanceBeforeUnits int64     `gorm:"column:balance_before_units"`
	BalanceAfterUnits  int64     `gorm:"column:balance_after_units"`
	SourceType         string    `gorm:"column:source_type"`
	SourceID           int64     `gorm:"column:source_id"`
	Remark             string    `gorm:"column:remark"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Transaction) TableName() string { return "wallet_transactions" }

// WalletWithUser is the read model used by the admin wallet list.
type WalletWithUser struct {
	Wallet
	Username string `gorm:"column:username"`
	Phone    string `gorm:"column:phone"`
	Email    string `gorm:"column:email"`
}

// TransactionWithUser is the read model used by transaction/ledger lists.
type TransactionWithUser struct {
	Transaction
	Username string `gorm:"column:username"`
	Phone    string `gorm:"column:phone"`
	Email    string `gorm:"column:email"`
}
