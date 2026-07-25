package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"admin_back_go/internal/module/payment/wallet"
)

type listRequest struct {
	CurrentPage int        `form:"current_page"`
	PageSize    int        `form:"page_size"`
	BatchNo     string     `form:"batch_no"`
	State       string     `form:"state"`
	UsedBy      int64      `form:"used_by"`
	UsedUser    string     `form:"used_user"`
	CreatedBy   int64      `form:"created_by"`
	Note        string     `form:"note"`
	CreatedFrom *time.Time `form:"created_from"`
	CreatedTo   *time.Time `form:"created_to"`
	ExpiresFrom *time.Time `form:"expires_from"`
	ExpiresTo   *time.Time `form:"expires_to"`
}

type lookupRequest struct {
	Code nonNullString `json:"code"`
}

type exportRequest struct {
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

type generateBatchRequest struct {
	RequestID string     `json:"request_id"`
	Amount    string     `json:"amount"`
	Quantity  int        `json:"quantity"`
	ExpiresAt *time.Time `json:"expires_at"`
	Note      string     `json:"note"`
}

type voidRequest struct {
	IDs []int64 `json:"ids"`
}

type redemptionRequest struct {
	Code nonNullString `json:"code"`
}

type nonNullString string

func (value *nonNullString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("value must be a string")
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*value = nonNullString(decoded)
	return nil
}

type redemptionResponse struct {
	Amount      string                 `json:"amount"`
	Transaction redemptionTransaction  `json:"transaction"`
	Wallet      wallet.SummaryResponse `json:"wallet"`
	Replayed    bool                   `json:"replayed"`
}

type redemptionTransaction struct {
	TransactionNo      string `json:"transaction_no"`
	Direction          string `json:"direction"`
	DirectionText      string `json:"direction_text"`
	Amount             string `json:"amount"`
	BalanceBefore      string `json:"balance_before"`
	BalanceAfter       string `json:"balance_after"`
	SourceType         string `json:"source_type"`
	SourceTypeText     string `json:"source_type_text"`
	CreatedAt          string `json:"created_at"`
}
