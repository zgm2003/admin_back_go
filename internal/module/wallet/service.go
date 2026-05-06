package wallet

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

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

func (s *Service) CreateAdjustment(ctx context.Context, input CreateAdjustmentInput) (*WalletAdjustmentCreateResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	mutation, appErr := normalizeAdjustmentInput(input)
	if appErr != nil {
		return nil, appErr
	}
	result, err := repo.CreateAdjustment(ctx, mutation)
	if err != nil {
		return nil, mapAdjustmentError(err)
	}
	return &WalletAdjustmentCreateResponse{
		TransactionID: result.TransactionID,
		BizActionNo:   result.BizActionNo,
		BalanceBefore: result.BalanceBefore,
		BalanceAfter:  result.BalanceAfter,
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

func normalizeAdjustmentInput(input CreateAdjustmentInput) (AdjustmentMutation, *apperror.Error) {
	reason := strings.TrimSpace(input.Reason)
	key := strings.TrimSpace(input.IdempotencyKey)
	if input.UserID <= 0 {
		return AdjustmentMutation{}, apperror.BadRequest("无效的用户ID")
	}
	if input.OperatorID <= 0 {
		return AdjustmentMutation{}, apperror.BadRequest("未获取到操作人")
	}
	if input.Delta == 0 {
		return AdjustmentMutation{}, apperror.BadRequest("调整金额不能为0")
	}
	if reason == "" || len([]rune(reason)) > 255 {
		return AdjustmentMutation{}, apperror.BadRequest("调账原因不能为空且不能超过255个字符")
	}
	if key == "" || len(key) < 8 || len(key) > 50 || !idempotencyKeyPattern.MatchString(key) {
		return AdjustmentMutation{}, apperror.BadRequest("幂等键格式错误")
	}
	return AdjustmentMutation{
		UserID:         input.UserID,
		Delta:          input.Delta,
		Reason:         reason,
		IdempotencyKey: key,
		BizActionNo:    "WALLET:ADJUST:" + key,
		OperatorID:     input.OperatorID,
	}, nil
}

func mapAdjustmentError(err error) *apperror.Error {
	switch {
	case errors.Is(err, ErrUserNotFound):
		return apperror.NotFound("用户不存在")
	case errors.Is(err, ErrInsufficientBalance):
		return apperror.BadRequest("可用余额不足，无法调减")
	case errors.Is(err, ErrAdjustmentConflict):
		return apperror.BadRequest("幂等键已被不同请求使用")
	case errors.Is(err, ErrRepositoryNotConfigured):
		return apperror.Internal("钱包仓储未配置")
	default:
		return apperror.Wrap(apperror.CodeInternal, 500, "钱包调账失败", err)
	}
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
