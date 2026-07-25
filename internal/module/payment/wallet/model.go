package wallet

import "time"

// Wallet maps user_wallets and owns balance facts for wallet APIs.
type Wallet struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	UserID             int64     `gorm:"column:user_id"`
	BalanceUnits       int64     `gorm:"column:balance_units"`
	TotalRechargeUnits int64     `gorm:"column:total_recharge_units"`
	TotalConsumeUnits  int64     `gorm:"column:total_consume_units"`
	HeldUnits          int64     `gorm:"column:held_units"`
	IsDel              int       `gorm:"column:is_del"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (Wallet) TableName() string { return "user_wallets" }

// Transaction maps wallet_transactions. AmountUnits is always positive;
// Direction tells whether the money moves in or out.
type Transaction struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	TransactionNo      string    `gorm:"column:transaction_no"`
	WalletID           int64     `gorm:"column:wallet_id"`
	UserID             int64     `gorm:"column:user_id"`
	Direction          string    `gorm:"column:direction"`
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

// Hold is the immutable Run-level reservation state.
type Hold struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	WalletID      int64     `gorm:"column:wallet_id"`
	UserID        int64     `gorm:"column:user_id"`
	RunID         int64     `gorm:"column:run_id"`
	HeldUnits     int64     `gorm:"column:held_units"`
	CapturedUnits int64     `gorm:"column:captured_units"`
	Status        string    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Hold) TableName() string { return "wallet_holds" }
