package wallet

import "admin_back_go/internal/shared/dict"

const (
	DirectionIn  = "in"
	DirectionOut = "out"

	SourceRecharge   = "recharge"
	SourceAIGenerate = "ai_generate"
	SourceAIRefund   = "ai_refund"

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
	BalanceCents       int64  `json:"balance_cents"`
	BalanceText        string `json:"balance_text"`
	TotalRechargeCents int64  `json:"total_recharge_cents"`
	TotalRechargeText  string `json:"total_recharge_text"`
	TotalConsumeCents  int64  `json:"total_consume_cents"`
	TotalConsumeText   string `json:"total_consume_text"`
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
	ID                 int64  `json:"id"`
	TransactionNo      string `json:"transaction_no"`
	UserID             int64  `json:"user_id"`
	Username           string `json:"username"`
	Account            string `json:"account"`
	Direction          string `json:"direction"`
	DirectionText      string `json:"direction_text"`
	AmountCents        int64  `json:"amount_cents"`
	AmountText         string `json:"amount_text"`
	BalanceBeforeCents int64  `json:"balance_before_cents"`
	BalanceBeforeText  string `json:"balance_before_text"`
	BalanceAfterCents  int64  `json:"balance_after_cents"`
	BalanceAfterText   string `json:"balance_after_text"`
	SourceType         string `json:"source_type"`
	SourceTypeText     string `json:"source_type_text"`
	SourceID           int64  `json:"source_id"`
	Remark             string `json:"remark"`
	CreatedAt          string `json:"created_at"`
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
	ID                 int64  `json:"id"`
	WalletID           int64  `json:"wallet_id"`
	UserID             int64  `json:"user_id"`
	Username           string `json:"username"`
	Account            string `json:"account"`
	BalanceCents       int64  `json:"balance_cents"`
	BalanceText        string `json:"balance_text"`
	TotalRechargeCents int64  `json:"total_recharge_cents"`
	TotalRechargeText  string `json:"total_recharge_text"`
	TotalConsumeCents  int64  `json:"total_consume_cents"`
	TotalConsumeText   string `json:"total_consume_text"`
	UpdatedAt          string `json:"updated_at"`
}

type MutationInput struct {
	UserID      int64
	AmountCents int64
	SourceType  string
	SourceID    int64
	Remark      string
}

type MutationResponse struct {
	Transaction TransactionItem `json:"transaction"`
	Wallet      SummaryResponse `json:"wallet"`
}
