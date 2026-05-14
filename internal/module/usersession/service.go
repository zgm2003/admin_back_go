package usersession

import (
	"context"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/session"
)

const timeLayout = "2006-01-02 15:04:05"

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Stats(ctx context.Context) (*StatsResponse, *apperror.Error)
	Revoke(ctx context.Context, id int64, currentSessionID int64) (*RevokeResponse, *apperror.Error)
	BatchRevoke(ctx context.Context, input BatchRevokeInput, currentSessionID int64) (*BatchRevokeResponse, *apperror.Error)
}

type OptionFunc func(*Service)

type CacheRevoker interface {
	RevokeCache(ctx context.Context, row session.Session) error
	RevokeCaches(ctx context.Context, rows []session.Session) error
}

type Service struct {
	repository   Repository
	cacheRevoker CacheRevoker
	now          func() time.Time
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

func WithCacheRevoker(revoker CacheRevoker) OptionFunc {
	return func(s *Service) {
		s.cacheRevoker = revoker
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
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
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
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.stats_failed", nil, "查询用户会话统计失败", err)
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

func (s *Service) Revoke(ctx context.Context, id int64, currentSessionID int64) (*RevokeResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if id <= 0 {
		return nil, apperror.BadRequestKey("usersession.id.invalid", nil, "无效的用户会话ID")
	}
	if id == currentSessionID {
		return nil, apperror.BadRequestKey("usersession.revoke_current_forbidden", nil, "不能踢下线当前会话")
	}
	row, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}
	if row == nil {
		return nil, apperror.NotFoundKey("usersession.not_found", nil, "用户会话不存在")
	}
	if row.RevokedAt != nil {
		return &RevokeResponse{ID: id, Revoked: false}, nil
	}
	now := s.now()
	if _, err := repo.MarkRevoked(ctx, []int64{id}, now); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.revoke_failed", nil, "踢下线用户会话失败", err)
	}
	if err := s.revokeCache(ctx, *row); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.cache_revoke_failed", nil, "清理用户会话缓存失败", err)
	}
	return &RevokeResponse{ID: id, Revoked: true}, nil
}

func (s *Service) BatchRevoke(ctx context.Context, input BatchRevokeInput, currentSessionID int64) (*BatchRevokeResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	ids := normalizeIDs(input.IDs)
	if len(ids) > 100 {
		return nil, apperror.BadRequestKey("usersession.batch_too_many", nil, "单次最多踢下线100个会话")
	}
	if len(ids) == 0 {
		return &BatchRevokeResponse{}, nil
	}
	rows, err := repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}

	response := &BatchRevokeResponse{}
	toRevoke := make([]SessionRecord, 0, len(rows))
	revokeIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ID == currentSessionID {
			response.SkippedCurrent++
			continue
		}
		if row.RevokedAt != nil {
			response.SkippedAlreadyRevoked++
			continue
		}
		toRevoke = append(toRevoke, row)
		revokeIDs = append(revokeIDs, row.ID)
	}
	if len(revokeIDs) == 0 {
		return response, nil
	}
	count, err := repo.MarkRevoked(ctx, revokeIDs, s.now())
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.batch_revoke_failed", nil, "批量踢下线用户会话失败", err)
	}
	if err := s.revokeCaches(ctx, toRevoke); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.cache_revoke_failed", nil, "清理用户会话缓存失败", err)
	}
	response.Count = count
	return response, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("usersession.repository_missing", nil, "用户会话仓储未配置")
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
		return query, apperror.BadRequestKey("usersession.platform.invalid", nil, "无效的平台标识")
	}
	if query.Status != "" && !isSessionStatus(query.Status) {
		return query, apperror.BadRequestKey("usersession.status.invalid", nil, "无效的会话状态")
	}
	query.Now = s.now()
	return query, nil
}

func (s *Service) revokeCache(ctx context.Context, row SessionRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	return s.cacheRevoker.RevokeCache(ctx, sessionRow(row))
}

func (s *Service) revokeCaches(ctx context.Context, rows []SessionRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	sessions := make([]session.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessionRow(row))
	}
	return s.cacheRevoker.RevokeCaches(ctx, sessions)
}

func sessionRow(row SessionRecord) session.Session {
	return session.Session{ID: row.ID, UserID: row.UserID, Platform: row.Platform, AccessTokenHash: row.AccessTokenHash}
}

func normalizeIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
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
