package redeemcode

import (
	"time"

	"admin_back_go/internal/module/payment/wallet"
)

const (
	StateUnused  = "unused"
	StateUsed    = "used"
	StateVoided  = "voided"
	StateExpired = "expired"
)

type Batch struct {
	ID                        int64      `gorm:"column:id;primaryKey"`
	BatchNo                   string     `gorm:"column:batch_no"`
	RequestID                 string     `gorm:"column:request_id"`
	RequestFingerprintVersion string     `gorm:"column:request_fingerprint_version"`
	RequestFingerprint        string     `gorm:"column:request_fingerprint"`
	AmountCents               int64      `gorm:"column:amount_cents"`
	Quantity                  int        `gorm:"column:quantity"`
	ExpiresAt                 *time.Time `gorm:"column:expires_at"`
	Note                      string     `gorm:"column:note"`
	CreatedBy                 int64      `gorm:"column:created_by"`
	CreatedAt                 time.Time  `gorm:"column:created_at"`
	UpdatedAt                 time.Time  `gorm:"column:updated_at"`
}

func (Batch) TableName() string { return "redeem_code_batches" }

type Code struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	BatchID   int64      `gorm:"column:batch_id"`
	Code      string     `gorm:"column:code"`
	State     string     `gorm:"column:state"`
	UsedBy    *int64     `gorm:"column:used_by"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (Code) TableName() string { return "redeem_codes" }

type BatchWithCodes struct {
	Batch Batch
	Codes []Code
}

type CreateBatchRecord struct {
	Batch Batch
	Codes []Code
}

type CodeView struct {
	ID                  int64      `gorm:"column:id"`
	BatchID             int64      `gorm:"column:batch_id"`
	Code                string     `gorm:"column:code"`
	State               string     `gorm:"column:state"`
	UsedBy              *int64     `gorm:"column:used_by"`
	UsedAt              *time.Time `gorm:"column:used_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	BatchNo             string     `gorm:"column:batch_no"`
	AmountCents         int64      `gorm:"column:amount_cents"`
	ExpiresAt           *time.Time `gorm:"column:expires_at"`
	Note                string     `gorm:"column:note"`
	CreatedBy           int64      `gorm:"column:created_by"`
	CreatorUsername     string     `gorm:"column:creator_username"`
	UsedUsername        string     `gorm:"column:used_username"`
	UsedAccount         string     `gorm:"column:used_account"`
	WalletTransactionNo string     `gorm:"column:wallet_transaction_no"`
}

type RedemptionFact struct {
	AmountCents int64
	Transaction *wallet.Transaction
	Wallet      *wallet.Wallet
	Replayed    bool
}
