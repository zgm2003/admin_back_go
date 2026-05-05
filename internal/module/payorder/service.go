package payorder

import (
	"context"
	"encoding/json"
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
	now        func() time.Time
}

type Option func(*Service)

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		ChannelArr:        dict.PayChannelOptions(),
		PayMethodArr:      dict.PayMethodOptions(),
		OrderTypeArr:      dict.PayOrderTypeOptions(),
		PayStatusArr:      dict.PayStatusOptions(),
		BizStatusArr:      dict.PayBizStatusOptions(),
		RechargePresetArr: dict.RechargePresetOptions(),
	}}, nil
}

func (s *Service) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.OrderNo = strings.TrimSpace(query.OrderNo)
	counts, err := repo.CountByStatus(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询订单状态统计失败", err)
	}
	options := dict.PayStatusOptions()
	result := make([]StatusCountItem, 0, len(options))
	for _, option := range options {
		result = append(result, StatusCountItem{Label: option.Label, Value: option.Value, Count: counts[option.Value]})
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询订单失败", err)
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

func (s *Service) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的订单ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Detail(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询订单详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("订单不存在")
	}
	items, err := repo.Items(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询订单明细失败", err)
	}
	return detailResponse(*row, items), nil
}

func (s *Service) Remark(ctx context.Context, id int64, input RemarkInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的订单ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	remark := strings.TrimSpace(input.Remark)
	if remark == "" || len([]rune(remark)) > 500 {
		return apperror.BadRequest("备注不能为空且不能超过500个字符")
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询订单失败", err)
	}
	if row == nil {
		return apperror.NotFound("订单不存在")
	}
	affected, err := repo.UpdateRemark(ctx, id, remark)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新订单备注失败", err)
	}
	if affected == 0 {
		return apperror.BadRequest("订单状态已变更，请刷新后重试")
	}
	return nil
}

func (s *Service) Close(ctx context.Context, id int64, input CloseInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的订单ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "管理员关闭"
	}
	if len([]rune(reason)) > 100 {
		return apperror.BadRequest("关闭原因不能超过100个字符")
	}
	now := input.Now
	if now.IsZero() {
		now = s.now()
	}

	var closeErr *apperror.Error
	err := repo.WithTx(ctx, func(tx Repository) error {
		row, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if row == nil {
			closeErr = apperror.NotFound("订单不存在")
			return nil
		}
		if row.PayStatus != enum.PayStatusPending && row.PayStatus != enum.PayStatusPaying {
			closeErr = apperror.BadRequest("该订单状态不允许关闭")
			return nil
		}
		affected, err := tx.CloseOrder(ctx, id, row.PayStatus, reason, now)
		if err != nil {
			return err
		}
		if affected == 0 {
			closeErr = apperror.BadRequest("订单状态已变更，请刷新后重试")
			return nil
		}
		_, err = tx.CloseLastActiveTransaction(ctx, row.ID, now)
		return err
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "关闭订单失败", err)
	}
	return closeErr
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("订单仓储未配置")
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
	query.OrderNo = strings.TrimSpace(query.OrderNo)
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, OrderNo: row.OrderNo, UserID: row.UserID, UserName: row.UserName, UserEmail: row.UserEmail,
		OrderType: row.OrderType, OrderTypeText: enum.PayOrderTypeLabels[row.OrderType], Title: row.Title,
		TotalAmount: row.TotalAmount, DiscountAmount: row.DiscountAmount, PayAmount: row.PayAmount,
		PayStatus: row.PayStatus, PayStatusText: enum.PayStatusLabels[row.PayStatus], BizStatus: row.BizStatus,
		BizStatusText: enum.PayBizStatusLabels[row.BizStatus], AdminRemark: row.AdminRemark,
		PayTime: formatOptionalTime(row.PayTime), CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailResponse(row DetailRow, rows []OrderItem) *DetailResponse {
	order := DetailOrder{
		ID: row.ID, OrderNo: row.OrderNo, UserID: row.UserID, UserName: row.UserName, UserEmail: row.UserEmail,
		OrderType: row.OrderType, OrderTypeText: enum.PayOrderTypeLabels[row.OrderType], BizType: row.BizType,
		BizID: row.BizID, Title: row.Title, TotalAmount: row.TotalAmount, DiscountAmount: row.DiscountAmount,
		PayAmount: row.PayAmount, PayStatus: row.PayStatus, PayStatusText: enum.PayStatusLabels[row.PayStatus],
		BizStatus: row.BizStatus, BizStatusText: enum.PayBizStatusLabels[row.BizStatus], PayTime: formatOptionalTime(row.PayTime),
		ExpireTime: formatOptionalTime(&row.ExpireTime), CloseTime: formatOptionalTime(row.CloseTime), CloseReason: row.CloseReason,
		BizDoneAt: formatOptionalTime(row.BizDoneAt), AdminRemark: row.AdminRemark, PayMethod: row.PayMethod,
		Extra: normalizeJSON(row.Extra), SuccessTransactionID: row.SuccessTransactionID, CreatedAt: formatTime(row.CreatedAt),
	}
	if row.ChannelID > 0 {
		order.Channel = &ChannelSummary{ID: row.ChannelID, Name: row.ChannelName, Channel: row.Channel}
	}
	items := make([]Item, 0, len(rows))
	for _, item := range rows {
		items = append(items, Item{ID: item.ID, Title: item.Title, Price: item.Price, Quantity: item.Quantity, Amount: item.Amount})
	}
	return &DetailResponse{Order: order, Items: items}
}

func normalizeJSON(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.Format(timeLayout)
	return &text
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
