package wallet

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	WalletTypeArr   []dict.Option[int] `json:"wallet_type_arr"`
	WalletSourceArr []dict.Option[int] `json:"wallet_source_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      *int64
	StartDate   string
	EndDate     string
}

type TransactionListQuery struct {
	CurrentPage int
	PageSize    int
	UserID      *int64
	Type        *int
	StartDate   string
	EndDate     string
}

type CreateAdjustmentInput struct {
	UserID         int64
	Delta          int
	Reason         string
	IdempotencyKey string
	OperatorID     int64
}

type AdjustmentMutation struct {
	UserID         int64
	Delta          int
	Reason         string
	IdempotencyKey string
	BizActionNo    string
	OperatorID     int64
}

type AdjustmentResult struct {
	TransactionID int64
	BizActionNo   string
	BalanceBefore int
	BalanceAfter  int
}

type WalletAdjustmentCreateResponse struct {
	TransactionID int64  `json:"transaction_id"`
	BizActionNo   string `json:"biz_action_no"`
	BalanceBefore int    `json:"balance_before"`
	BalanceAfter  int    `json:"balance_after"`
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	UserName      string `json:"user_name"`
	UserEmail     string `json:"user_email"`
	Balance       int    `json:"balance"`
	Frozen        int    `json:"frozen"`
	TotalRecharge int    `json:"total_recharge"`
	TotalConsume  int    `json:"total_consume"`
	CreatedAt     string `json:"created_at"`
}

type TransactionListResponse struct {
	List []TransactionItem `json:"list"`
	Page Page              `json:"page"`
}

type TransactionItem struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	UserName       string `json:"user_name"`
	UserEmail      string `json:"user_email"`
	BizActionNo    string `json:"biz_action_no"`
	Type           int    `json:"type"`
	TypeText       string `json:"type_text"`
	AvailableDelta int    `json:"available_delta"`
	FrozenDelta    int    `json:"frozen_delta"`
	BalanceBefore  int    `json:"balance_before"`
	BalanceAfter   int    `json:"balance_after"`
	OrderNo        string `json:"order_no"`
	Title          string `json:"title"`
	Remark         string `json:"remark"`
	CreatedAt      string `json:"created_at"`
}

type ListRow struct {
	ID            int64
	UserID        int64
	UserName      string
	UserEmail     string
	Balance       int
	Frozen        int
	TotalRecharge int
	TotalConsume  int
	CreatedAt     time.Time
}

type TransactionRow struct {
	ID             int64
	UserID         int64
	UserName       string
	UserEmail      string
	BizActionNo    string
	Type           int
	AvailableDelta int
	FrozenDelta    int
	BalanceBefore  int
	BalanceAfter   int
	OrderNo        string
	Title          string
	Remark         string
	CreatedAt      time.Time
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error)
	CreateAdjustment(ctx context.Context, input CreateAdjustmentInput) (*WalletAdjustmentCreateResponse, *apperror.Error)
}
