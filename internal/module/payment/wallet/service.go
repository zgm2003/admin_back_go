package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return NewServiceWithNow(repository, time.Now)
}

func NewServiceWithNow(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

func (s *Service) Summary(ctx context.Context, userID int64) (*SummaryResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	wallet, err := repo.GetOrCreateWallet(ctx, userID)
	if err != nil {
		return nil, wrapInternal("wallet.summary.query_failed", "查询钱包失败", err)
	}
	return summaryResponse(wallet), nil
}

func (s *Service) Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	if query.UserID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	return s.listTransactions(ctx, normalizeTransactionQuery(query))
}

func (s *Service) WalletUsersPageInit(ctx context.Context) (*WalletUsersPageInitResponse, *apperror.Error) {
	_ = ctx
	return &WalletUsersPageInitResponse{}, nil
}

func (s *Service) WalletUsers(ctx context.Context, query WalletUserListQuery) (*WalletUserListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	current, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	query.CurrentPage = current
	query.PageSize = size
	rows, total, err := repo.ListWalletUsers(ctx, query)
	if err != nil {
		return nil, wrapInternal("wallet.users.query_failed", "查询用户钱包失败", err)
	}
	items := make([]WalletUserItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, walletUserItem(row))
	}
	return &WalletUserListResponse{List: items, Page: Page{CurrentPage: current, PageSize: size, TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) LedgerPageInit(ctx context.Context) (*LedgerPageInitResponse, *apperror.Error) {
	_ = ctx
	return &LedgerPageInitResponse{Dict: walletDict()}, nil
}

func (s *Service) Ledger(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	return s.listTransactions(ctx, normalizeTransactionQuery(query))
}

func (s *Service) Debit(ctx context.Context, input MutationInput) (*MutationResponse, *apperror.Error) {
	return s.mutate(ctx, input, DirectionOut)
}

func (s *Service) Credit(ctx context.Context, input MutationInput) (*MutationResponse, *apperror.Error) {
	return s.mutate(ctx, input, DirectionIn)
}

func (s *Service) mutate(ctx context.Context, input MutationInput, direction string) (*MutationResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	input.Remark = strings.TrimSpace(input.Remark)
	prefix := "wallet.credit"
	if direction == DirectionOut {
		prefix = "wallet.debit"
	}
	if input.UserID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if input.AmountCents <= 0 {
		return nil, apperror.BadRequestKey(prefix+".amount.invalid", nil, "钱包变动金额必须大于0")
	}
	if !validMutationSource(direction, input.SourceType) {
		return nil, apperror.BadRequestKey(prefix+".source_type.invalid", nil, "钱包变动来源类型无效")
	}
	if input.SourceID <= 0 {
		return nil, apperror.BadRequestKey(prefix+".source_id.invalid", nil, "钱包变动来源ID必须大于0")
	}

	var wallet *Wallet
	var tx *Transaction
	var err error
	if direction == DirectionOut {
		wallet, tx, err = repo.Debit(ctx, input, s.now())
	} else {
		wallet, tx, err = repo.Credit(ctx, input, s.now())
	}
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			return nil, apperror.BadRequestKey("wallet.debit.insufficient_balance", nil, "余额不足")
		}
		if errors.Is(err, ErrMutationSourceOwnerMismatch) {
			return nil, apperror.BadRequestKey("wallet.mutation.source_id.owner_mismatch", nil, "资金变动来源已被其他用户使用")
		}
		return nil, wrapInternal("wallet.mutation.failed", "钱包资金变动失败", err)
	}
	return &MutationResponse{Transaction: transactionItem(TransactionWithUser{Transaction: *tx}), Wallet: *summaryResponse(wallet)}, nil
}

func validMutationSource(direction string, sourceType string) bool {
	switch direction {
	case DirectionOut:
		return sourceType == SourceAIGenerate
	case DirectionIn:
		return sourceType == SourceAIRefund
	default:
		return false
	}
}

func (s *Service) listTransactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	current, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	query.CurrentPage = current
	query.PageSize = size
	rows, total, err := repo.ListTransactions(ctx, query)
	if err != nil {
		return nil, wrapInternal("wallet.transactions.query_failed", "查询资金流水失败", err)
	}
	items := make([]TransactionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, transactionItem(row))
	}
	return &TransactionListResponse{List: items, Page: Page{CurrentPage: current, PageSize: size, TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("wallet.service_missing", nil, "钱包服务未配置")
	}
	return s.repository, nil
}

func normalizeTransactionQuery(query TransactionListQuery) TransactionListQuery {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Direction = strings.TrimSpace(query.Direction)
	query.SourceType = strings.TrimSpace(query.SourceType)
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)
	current, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	query.CurrentPage = current
	query.PageSize = size
	return query
}

func summaryResponse(wallet *Wallet) *SummaryResponse {
	if wallet == nil {
		return &SummaryResponse{}
	}
	return &SummaryResponse{
		BalanceCents:       wallet.BalanceCents,
		BalanceText:        amountText(wallet.BalanceCents),
		TotalRechargeCents: wallet.TotalRechargeCents,
		TotalRechargeText:  amountText(wallet.TotalRechargeCents),
		TotalConsumeCents:  wallet.TotalConsumeCents,
		TotalConsumeText:   amountText(wallet.TotalConsumeCents),
	}
}

func transactionItem(row TransactionWithUser) TransactionItem {
	return TransactionItem{
		ID:                 row.ID,
		TransactionNo:      row.TransactionNo,
		UserID:             row.UserID,
		Username:           row.Username,
		Account:            accountText(row.Username, row.Phone, row.Email),
		Direction:          row.Direction,
		DirectionText:      directionText(row.Direction),
		AmountCents:        row.AmountCents,
		AmountText:         amountText(row.AmountCents),
		BalanceBeforeCents: row.BalanceBeforeCents,
		BalanceBeforeText:  amountText(row.BalanceBeforeCents),
		BalanceAfterCents:  row.BalanceAfterCents,
		BalanceAfterText:   amountText(row.BalanceAfterCents),
		SourceType:         row.SourceType,
		SourceTypeText:     sourceTypeText(row.SourceType),
		SourceID:           row.SourceID,
		Remark:             row.Remark,
		CreatedAt:          formatTime(row.CreatedAt),
	}
}

func walletUserItem(row WalletWithUser) WalletUserItem {
	return WalletUserItem{
		ID:                 row.ID,
		WalletID:           row.ID,
		UserID:             row.UserID,
		Username:           row.Username,
		Account:            accountText(row.Username, row.Phone, row.Email),
		BalanceCents:       row.BalanceCents,
		BalanceText:        amountText(row.BalanceCents),
		TotalRechargeCents: row.TotalRechargeCents,
		TotalRechargeText:  amountText(row.TotalRechargeCents),
		TotalConsumeCents:  row.TotalConsumeCents,
		TotalConsumeText:   amountText(row.TotalConsumeCents),
		UpdatedAt:          formatTime(row.UpdatedAt),
	}
}

func accountText(username, phone, email string) string {
	for _, value := range []string{phone, email, username} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func directionText(value string) string {
	switch value {
	case DirectionIn:
		return "收入"
	case DirectionOut:
		return "支出"
	default:
		return value
	}
}

func sourceTypeText(value string) string {
	switch value {
	case SourceRecharge:
		return "充值"
	case SourceAIGenerate:
		return "AI 生成"
	case SourceAIRefund:
		return "AI 退款"
	case SourceRedeemCode:
		return "兑换码充值"
	default:
		return value
	}
}

func walletDict() WalletDict {
	return WalletDict{
		DirectionArr:  []dict.Option[string]{{Label: "收入", Value: DirectionIn}, {Label: "支出", Value: DirectionOut}},
		SourceTypeArr: []dict.Option[string]{{Label: "充值", Value: SourceRecharge}, {Label: "AI生成", Value: SourceAIGenerate}, {Label: "AI退款", Value: SourceAIRefund}, {Label: "兑换码充值", Value: SourceRedeemCode}},
	}
}

func amountText(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func totalPage(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	pages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		pages++
	}
	return pages
}

func wrapInternal(key string, fallback string, err error) *apperror.Error {
	return apperror.WrapKey(apperror.CodeInternal, 500, key, nil, fallback, err)
}
