package redeemcode

import (
	"time"

	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/dict"
)

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type PageInitResponse struct {
	States []dict.Option[string] `json:"states"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	BatchNo     string
	State       string
	UsedBy      int64
	UsedUser    string
	CreatedBy   int64
	Note        string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	ExpiresFrom *time.Time
	ExpiresTo   *time.Time
}

type ListResponse struct {
	List []CodeItem `json:"list"`
	Page Page       `json:"page"`
}

type CodeItem struct {
	ID                  int64  `json:"id"`
	Code                string `json:"code"`
	BatchID             int64  `json:"batch_id"`
	BatchNo             string `json:"batch_no"`
	AmountCents         int64  `json:"amount_cents"`
	Amount              string `json:"amount"`
	State               string `json:"state"`
	ExpiresAt           string `json:"expires_at"`
	Note                string `json:"note"`
	UsedBy              int64  `json:"used_by"`
	UsedUsername        string `json:"used_username"`
	UsedAccount         string `json:"used_account"`
	UsedAt              string `json:"used_at"`
	CreatedBy           int64  `json:"created_by"`
	CreatorUsername     string `json:"creator_username"`
	CreatedAt           string `json:"created_at"`
	WalletTransactionNo string `json:"wallet_transaction_no"`
}

type LookupInput struct {
	Code string `json:"code"`
}

type LookupResponse struct {
	Item *CodeItem `json:"item"`
}

type GenerateBatchInput struct {
	RequestID string     `json:"request_id"`
	Amount    string     `json:"amount"`
	Quantity  int        `json:"quantity"`
	ExpiresAt *time.Time `json:"expires_at"`
	Note      string     `json:"note"`
}

type GenerateBatchResponse struct {
	Batch GeneratedBatchItem  `json:"batch"`
	Codes []GeneratedCodeItem `json:"codes"`
}

type GeneratedBatchItem struct {
	ID        int64  `json:"id"`
	BatchNo   string `json:"batch_no"`
	RequestID string `json:"request_id"`
	Amount    string `json:"amount"`
	Quantity  int    `json:"quantity"`
	ExpiresAt string `json:"expires_at"`
	Note      string `json:"note"`
	CreatedBy int64  `json:"created_by"`
	CreatedAt string `json:"created_at"`
	Replayed  bool   `json:"replayed"`
}

type GeneratedCodeItem struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
}

type VoidInput struct {
	IDs []int64 `json:"ids"`
}

type VoidResponse struct {
	Voided int `json:"voided"`
}

type ExportInput struct {
	BatchNo     string     `json:"batch_no"`
	State       string     `json:"state"`
	UsedBy      int64      `json:"used_by"`
	UsedUser    string     `json:"used_user"`
	CreatedBy   int64      `json:"created_by"`
	Note        string     `json:"note"`
	CreatedFrom *time.Time `json:"created_from"`
	CreatedTo   *time.Time `json:"created_to"`
	ExpiresFrom *time.Time `json:"expires_from"`
	ExpiresTo   *time.Time `json:"expires_to"`
}

func (input ExportInput) ListQuery() ListQuery {
	return ListQuery{
		BatchNo: input.BatchNo, State: input.State, UsedBy: input.UsedBy, UsedUser: input.UsedUser,
		CreatedBy: input.CreatedBy, Note: input.Note, CreatedFrom: input.CreatedFrom, CreatedTo: input.CreatedTo,
		ExpiresFrom: input.ExpiresFrom, ExpiresTo: input.ExpiresTo,
	}
}

type ExportResponse struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	RowCount int    `json:"row_count"`
}

type RedemptionResponse struct {
	Amount      string                 `json:"amount"`
	Transaction wallet.TransactionItem `json:"transaction"`
	Wallet      wallet.SummaryResponse `json:"wallet"`
	Replayed    bool                   `json:"replayed"`
}
