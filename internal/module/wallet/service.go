package wallet

import (
	"context"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		WalletTypeArr:   dict.WalletTypeOptions(),
		WalletSourceArr: dict.WalletSourceOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询钱包列表失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Transactions(ctx context.Context, query TransactionListQuery) (*TransactionListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeTransactionListQuery(query)
	rows, total, err := repo.Transactions(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询钱包流水失败", err)
	}
	list := make([]TransactionItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, transactionItem(row))
	}
	return &TransactionListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("钱包仓储未配置")
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func normalizeTransactionListQuery(query TransactionListQuery) TransactionListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, UserID: row.UserID, UserName: row.UserName, UserEmail: row.UserEmail,
		Balance: row.Balance, Frozen: row.Frozen, TotalRecharge: row.TotalRecharge, TotalConsume: row.TotalConsume,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func transactionItem(row TransactionRow) TransactionItem {
	return TransactionItem{
		ID: row.ID, UserID: row.UserID, UserName: row.UserName, UserEmail: row.UserEmail,
		BizActionNo: row.BizActionNo, Type: row.Type, TypeText: enum.WalletTypeLabels[row.Type],
		AvailableDelta: row.AvailableDelta, FrozenDelta: row.FrozenDelta, BalanceBefore: row.BalanceBefore,
		BalanceAfter: row.BalanceAfter, OrderNo: row.OrderNo, Title: row.Title, Remark: row.Remark,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
