package paytransaction

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
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		ChannelArr:   dict.PayChannelOptions(),
		TxnStatusArr: dict.PayTxnStatusOptions(),
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付流水失败", err)
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
		return nil, apperror.BadRequest("无效的支付流水ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Detail(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付流水详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("支付流水不存在")
	}
	return detailResponse(*row), nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付流水仓储未配置")
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
	query.TransactionNo = strings.TrimSpace(query.TransactionNo)
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, TransactionNo: row.TransactionNo, OrderNo: row.OrderNo, UserID: row.UserID,
		UserName: row.UserName, UserEmail: row.UserEmail, AttemptNo: row.AttemptNo, ChannelID: row.ChannelID,
		Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel], PayMethod: row.PayMethod,
		PayMethodText: payMethodText(row.PayMethod), Amount: row.Amount, TradeNo: row.TradeNo,
		TradeStatus: row.TradeStatus, Status: row.Status, StatusText: enum.PayTxnStatusLabels[row.Status],
		PaidAt: formatOptionalTime(row.PaidAt), CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailResponse(row DetailRow) *DetailResponse {
	response := &DetailResponse{
		Transaction: DetailTransaction{
			ID: row.ID, TransactionNo: row.TransactionNo, OrderNo: row.OrderNo, AttemptNo: row.AttemptNo,
			ChannelID: row.ChannelID, Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel],
			PayMethod: row.PayMethod, PayMethodText: payMethodText(row.PayMethod), Amount: row.Amount,
			TradeNo: row.TradeNo, TradeStatus: row.TradeStatus, Status: row.Status,
			StatusText: enum.PayTxnStatusLabels[row.Status], PaidAt: formatOptionalTime(row.PaidAt),
			ClosedAt: formatOptionalTime(row.ClosedAt), ChannelResp: normalizeJSON(row.ChannelResp),
			RawNotify: normalizeJSON(row.RawNotify), CreatedAt: formatTime(row.CreatedAt),
		},
	}
	if row.ChannelID > 0 {
		response.Channel = &ChannelSummary{ID: row.ChannelID, Name: row.PayChannelName, Channel: row.Channel}
	}
	if row.OrderID > 0 {
		response.Order = &OrderSummary{
			ID: row.OrderID, OrderNo: row.OrderNo, UserID: row.OrderUserID, UserName: row.OrderUserName,
			UserEmail: row.OrderUserEmail, Title: row.OrderTitle, PayAmount: row.OrderPayAmount, PayStatus: row.OrderPayStatus,
		}
	}
	return response
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

func payMethodText(method string) string {
	if label := enum.PayMethodLabels[method]; label != "" {
		return label
	}
	return method
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.Format(timeLayout)
	return &formatted
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
