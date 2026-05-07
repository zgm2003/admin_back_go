package paynotifylog

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
		ChannelArr:             dict.PayChannelOptions(),
		NotifyTypeArr:          dict.PayNotifyTypeOptions(),
		NotifyProcessStatusArr: dict.PayNotifyProcessStatusOptions(),
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付回调日志失败", err)
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
		return nil, apperror.BadRequest("无效的回调日志ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Detail(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付回调日志详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("回调日志不存在")
	}
	return &DetailResponse{Log: detailLog(*row)}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付回调日志仓储未配置")
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
	query.TransactionNo = strings.TrimSpace(query.TransactionNo)
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel],
		NotifyType: row.NotifyType, NotifyTypeText: enum.NotifyTypeLabels[row.NotifyType],
		TransactionNo: row.TransactionNo, TradeNo: row.TradeNo, ProcessStatus: row.ProcessStatus,
		ProcessStatusText: enum.NotifyProcessStatusLabels[row.ProcessStatus], ProcessMsg: row.ProcessMsg,
		IP: row.IP, CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailLog(row DetailRow) DetailLog {
	return DetailLog{
		ID: row.ID, Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel],
		NotifyType: row.NotifyType, NotifyTypeText: enum.NotifyTypeLabels[row.NotifyType],
		TransactionNo: row.TransactionNo, TradeNo: row.TradeNo, ProcessStatus: row.ProcessStatus,
		ProcessStatusText: enum.NotifyProcessStatusLabels[row.ProcessStatus], ProcessMsg: row.ProcessMsg,
		Headers: normalizeJSON(row.Headers), RawData: normalizeJSON(row.RawData), IP: row.IP,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
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
