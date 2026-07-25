package wallet

import "admin_back_go/internal/shared/dict"

const (
	DirectionIn  = "in"
	DirectionOut = "out"

	SourceRecharge   = "recharge"
	SourceAIGenerate = "ai_generate"
	SourceRedeemCode = "redeem_code"

	HoldActive   = "active"
	HoldCaptured = "captured"
	HoldReleased = "released"

	RechargeCreditCreated  RechargeCreditDisposition = "created"
	RechargeCreditReplayed RechargeCreditDisposition = "replayed"

	defaultPageSize = 20
	maxPageSize     = 100
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type SummaryResponse struct {
	Balance          string `json:"balance"`
	AvailableBalance string `json:"available_balance"`
	HeldAmount       string `json:"held_amount"`
	TotalRecharge    string `json:"total_recharge"`
	TotalConsume     string `json:"total_consume"`
}

type WalletUsersPageInitResponse struct{}

type LedgerPageInitResponse struct {
	Dict WalletDict `json:"dict"`
}

type WalletDict struct {
	DirectionArr  []dict.Option[string] `json:"direction_arr"`
	SourceTypeArr []dict.Option[string] `json:"source_type_arr"`
}

type TransactionListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      int64
	Keyword     string
	Direction   string
	SourceType  string
	DateStart   string
	DateEnd     string
}

type TransactionListResponse struct {
	List []TransactionItem `json:"list"`
	Page Page              `json:"page"`
}

type TransactionItem struct {
	ID             int64  `json:"id"`
	TransactionNo  string `json:"transaction_no"`
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	Account        string `json:"account"`
	Direction      string `json:"direction"`
	DirectionText  string `json:"direction_text"`
	Amount         string `json:"amount"`
	BalanceBefore  string `json:"balance_before"`
	BalanceAfter   string `json:"balance_after"`
	SourceType     string `json:"source_type"`
	SourceTypeText string `json:"source_type_text"`
	SourceID       int64  `json:"source_id"`
	Remark         string `json:"remark"`
	CreatedAt      string `json:"created_at"`
}

type WalletUserListQuery struct {
	CurrentPage int
	PageSize    int
	Keyword     string
	UserID      int64
}

type WalletUserListResponse struct {
	List []WalletUserItem `json:"list"`
	Page Page             `json:"page"`
}

type WalletUserItem struct {
	ID               int64  `json:"id"`
	WalletID         int64  `json:"wallet_id"`
	UserID           int64  `json:"user_id"`
	Username         string `json:"username"`
	Account          string `json:"account"`
	Balance          string `json:"balance"`
	TotalRecharge    string `json:"total_recharge"`
	TotalConsume     string `json:"total_consume"`
	AvailableBalance string `json:"available_balance"`
	HeldAmount       string `json:"held_amount"`
	UpdatedAt        string `json:"updated_at"`
}

type ReserveHoldInput struct{ UserID, RunID, AmountUnits int64 }
type TopUpHoldInput struct{ UserID, RunID, AmountUnits int64 }
type CaptureHoldInput struct {
	UserID, RunID, ActualUnits int64
	SourceSummary              string
}
type ReleaseHoldInput struct{ UserID, RunID int64 }
type CreditRechargeInput struct {
	UserID, RechargeID, AmountUnits int64
	Remark                          string
}

type RechargeCreditDisposition string

type RechargeCreditFact struct {
	Wallet      *Wallet
	Transaction *Transaction
	Disposition RechargeCreditDisposition
}

type RedeemCodeCreditInput struct {
	UserID      int64
	CodeID      int64
	AmountUnits int64
	BatchNo     string
}
