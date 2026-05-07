package usersession

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

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Stats(ctx context.Context) (*StatsResponse, *apperror.Error)
}

type OptionFunc func(*Service)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, opts ...OptionFunc) *Service {
	service := &Service{
		repository: repository,
		now:        time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithNow(now func() time.Time) OptionFunc {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		PlatformArr: []Option[string]{
			{Label: enum.PlatformAdmin, Value: enum.PlatformAdmin},
			{Label: enum.PlatformApp, Value: enum.PlatformApp},
		},
		StatusArr: []Option[string]{
			{Label: "在线", Value: SessionStatusActive},
			{Label: "已过期", Value: SessionStatusExpired},
			{Label: "已下线", Value: SessionStatusRevoked},
		},
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = s.normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户会话失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row, query.Now))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Stats(ctx context.Context) (*StatsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.Stats(ctx, s.now())
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户会话统计失败", err)
	}
	dist := map[string]int64{
		enum.PlatformAdmin: 0,
		enum.PlatformApp:   0,
	}
	var total int64
	for _, row := range rows {
		if row.Platform == "" {
			continue
		}
		dist[row.Platform] = row.Total
		total += row.Total
	}
	return &StatsResponse{TotalActive: total, PlatformDistribution: dist}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户会话仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Username = strings.TrimSpace(query.Username)
	query.Platform = strings.TrimSpace(query.Platform)
	query.Status = strings.TrimSpace(query.Status)
	if query.Platform != "" && !enum.IsPlatform(query.Platform) {
		return query, apperror.BadRequest("无效的平台标识")
	}
	if query.Status != "" && !isSessionStatus(query.Status) {
		return query, apperror.BadRequest("无效的会话状态")
	}
	query.Now = s.now()
	return query, nil
}

func isSessionStatus(value string) bool {
	return value == SessionStatusActive || value == SessionStatusExpired || value == SessionStatusRevoked
}

func listItem(row ListRow, now time.Time) ListItem {
	return ListItem{
		ID: row.ID, UserID: row.UserID, Username: row.Username,
		Platform: row.Platform, PlatformName: platformName(row.Platform),
		DeviceID: row.DeviceID, IP: row.IP, UserAgent: row.UserAgent,
		LastSeenAt: formatTime(row.LastSeenAt), CreatedAt: formatTime(row.CreatedAt),
		ExpiresAt: formatTime(row.ExpiresAt), RefreshExpiresAt: formatTime(row.RefreshExpiresAt),
		RevokedAt: formatOptionalTime(row.RevokedAt), Status: sessionStatus(row, now),
	}
}

func sessionStatus(row ListRow, now time.Time) string {
	if row.RevokedAt != nil {
		return SessionStatusRevoked
	}
	if !row.RefreshExpiresAt.After(now) {
		return SessionStatusExpired
	}
	return SessionStatusActive
}

func platformName(platform string) string {
	for _, item := range dict.PlatformOptions() {
		if item.Value == platform {
			return item.Label
		}
	}
	return platform
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
