package auth

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Session admin management merged from usersession dto.go
const (
	SessionStatusActive  = "active"
	SessionStatusExpired = "expired"
	SessionStatusRevoked = "revoked"
)

type SessionOption[T string] struct {
	Label string `json:"label"`
	Value T      `json:"value"`
}

type SessionPageInitResponse struct {
	Dict SessionPageInitDict `json:"dict"`
}

type SessionPageInitDict struct {
	PlatformArr []SessionOption[string] `json:"platformArr"`
	StatusArr   []SessionOption[string] `json:"statusArr"`
}

type SessionListQuery struct {
	CurrentPage int
	PageSize    int
	Username    string
	Platform    string
	Status      string
	Now         time.Time
}

type SessionPage struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type SessionListResponse struct {
	List []SessionListItem `json:"list"`
	Page SessionPage       `json:"page"`
}

type SessionListItem struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Username         string  `json:"username"`
	Platform         string  `json:"platform"`
	PlatformName     string  `json:"platform_name"`
	DeviceID         string  `json:"device_id"`
	IP               string  `json:"ip"`
	UserAgent        string  `json:"ua"`
	LastSeenAt       string  `json:"last_seen_at"`
	CreatedAt        string  `json:"created_at"`
	ExpiresAt        string  `json:"expires_at"`
	RefreshExpiresAt string  `json:"refresh_expires_at"`
	RevokedAt        *string `json:"revoked_at"`
	Status           string  `json:"status"`
}

type SessionListRow struct {
	ID               int64
	UserID           int64
	Username         string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	CreatedAt        time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	RevokedAt        *time.Time
}

type SessionStatsResponse struct {
	TotalActive          int64            `json:"total_active"`
	PlatformDistribution map[string]int64 `json:"platform_distribution"`
}

type SessionStatsRow struct {
	Platform string
	Total    int64
}

type SessionRevokeResponse struct {
	ID      int64 `json:"id"`
	Revoked bool  `json:"revoked"`
}

type SessionBatchRevokeInput struct {
	IDs []int64
}

type SessionBatchRevokeResponse struct {
	Count                 int64 `json:"count"`
	SkippedCurrent        int   `json:"skipped_current"`
	SkippedAlreadyRevoked int   `json:"skipped_already_revoked"`
}

type SessionAdminRecord struct {
	ID              int64
	UserID          int64
	Platform        string
	AccessTokenHash string
	RevokedAt       *time.Time
}

// Session admin management merged from usersession repository.go
var ErrSessionAdminRepositoryNotConfigured = errors.New("user session repository is not configured")

type SessionAdminRepository interface {
	List(ctx context.Context, query SessionListQuery) ([]SessionListRow, int64, error)
	Stats(ctx context.Context, now time.Time) ([]SessionStatsRow, error)
	GetByID(ctx context.Context, id int64) (*SessionAdminRecord, error)
	GetByIDs(ctx context.Context, ids []int64) ([]SessionAdminRecord, error)
	MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error)
}

type SessionAdminGormRepository struct {
	db *gorm.DB
}

func NewSessionAdminGormRepository(client *database.Client) *SessionAdminGormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &SessionAdminGormRepository{db: client.Gorm}
}

func (r *SessionAdminGormRepository) List(ctx context.Context, query SessionListQuery) ([]SessionListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrSessionAdminRepositoryNotConfigured
	}
	query.Now = normalizeSessionAdminNow(query.Now)
	db := r.baseSessionListQuery(ctx, query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []SessionListRow
	err := db.Select(`
			us.id,
			us.user_id,
			COALESCE(u.username, '') AS username,
			us.platform,
			us.device_id,
			us.ip,
			COALESCE(us.ua, '') AS user_agent,
			us.last_seen_at,
			us.created_at,
			us.expires_at,
			us.refresh_expires_at,
			us.revoked_at
		`).
		Order(clause.Expr{
			SQL: `CASE
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at > ? THEN 1
				WHEN us.revoked_at IS NULL AND us.refresh_expires_at <= ? THEN 2
				ELSE 3
			END ASC`,
			Vars:               []any{query.Now, query.Now},
			WithoutParentheses: true,
		}).
		Order("us.last_seen_at DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *SessionAdminGormRepository) Stats(ctx context.Context, now time.Time) ([]SessionStatsRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	now = normalizeSessionAdminNow(now)
	var rows []SessionStatsRow
	err := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Where("us.is_del = ?", enum.CommonNo).
		Where("us.revoked_at IS NULL").
		Where("us.refresh_expires_at > ?", now).
		Select("us.platform, COUNT(*) AS total").
		Group("us.platform").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SessionAdminGormRepository) GetByID(ctx context.Context, id int64) (*SessionAdminRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	var row SessionAdminRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SessionAdminGormRepository) GetByIDs(ctx context.Context, ids []int64) ([]SessionAdminRecord, error) {
	if r == nil || r.db == nil {
		return nil, ErrSessionAdminRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return []SessionAdminRecord{}, nil
	}
	var rows []SessionAdminRecord
	err := r.db.WithContext(ctx).
		Table("user_sessions").
		Select("id, user_id, platform, access_token_hash, revoked_at").
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Order("id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SessionAdminGormRepository) MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrSessionAdminRepositoryNotConfigured
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&SessionAdminRecordModel{}).
		Where("id IN ?", ids).
		Where("revoked_at IS NULL").
		Where("is_del = ?", enum.CommonNo).
		Update("revoked_at", revokedAt)
	return result.RowsAffected, result.Error
}

func (r *SessionAdminGormRepository) baseSessionListQuery(ctx context.Context, query SessionListQuery) *gorm.DB {
	db := r.db.WithContext(ctx).
		Table("user_sessions AS us").
		Joins("LEFT JOIN users AS u ON u.id = us.user_id").
		Where("us.is_del = ?", enum.CommonNo)

	if query.Username != "" {
		db = db.Where("u.username LIKE ?", strings.TrimSpace(query.Username)+"%")
	}
	if query.Platform != "" {
		db = db.Where("us.platform = ?", query.Platform)
	}
	if query.Status != "" {
		switch query.Status {
		case SessionStatusActive:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at > ?", query.Now)
		case SessionStatusExpired:
			db = db.Where("us.revoked_at IS NULL").Where("us.refresh_expires_at <= ?", query.Now)
		case SessionStatusRevoked:
			db = db.Where("us.revoked_at IS NOT NULL")
		}
	}
	return db
}

func normalizeSessionAdminNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}

type SessionAdminRecordModel struct {
	ID        int64      `gorm:"column:id"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	IsDel     int        `gorm:"column:is_del"`
}

func (SessionAdminRecordModel) TableName() string {
	return "user_sessions"
}

// Session admin management merged from usersession service.go
const sessionAdminTimeLayout = "2006-01-02 15:04:05"

type SessionAdminHTTPService interface {
	PageInit(ctx context.Context) (*SessionPageInitResponse, *apperror.Error)
	List(ctx context.Context, query SessionListQuery) (*SessionListResponse, *apperror.Error)
	Stats(ctx context.Context) (*SessionStatsResponse, *apperror.Error)
	Revoke(ctx context.Context, id int64, currentSessionID int64) (*SessionRevokeResponse, *apperror.Error)
	BatchRevoke(ctx context.Context, input SessionBatchRevokeInput, currentSessionID int64) (*SessionBatchRevokeResponse, *apperror.Error)
}

type SessionAdminOption func(*SessionAdminService)

type SessionAdminCacheRevoker interface {
	RevokeCache(ctx context.Context, row Session) error
	RevokeCaches(ctx context.Context, rows []Session) error
}

type SessionAdminService struct {
	repository   SessionAdminRepository
	cacheRevoker SessionAdminCacheRevoker
	now          func() time.Time
}

func NewSessionAdminService(repository SessionAdminRepository, opts ...SessionAdminOption) *SessionAdminService {
	service := &SessionAdminService{
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

func WithSessionAdminNow(now func() time.Time) SessionAdminOption {
	return func(s *SessionAdminService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithSessionAdminCacheRevoker(revoker SessionAdminCacheRevoker) SessionAdminOption {
	return func(s *SessionAdminService) {
		s.cacheRevoker = revoker
	}
}

func (s *SessionAdminService) PageInit(ctx context.Context) (*SessionPageInitResponse, *apperror.Error) {
	return &SessionPageInitResponse{Dict: SessionPageInitDict{
		PlatformArr: []SessionOption[string]{
			{Label: enum.PlatformAdmin, Value: enum.PlatformAdmin},
			{Label: enum.PlatformApp, Value: enum.PlatformApp},
		},
		StatusArr: []SessionOption[string]{
			{Label: "在线", Value: SessionStatusActive},
			{Label: "已过期", Value: SessionStatusExpired},
			{Label: "已下线", Value: SessionStatusRevoked},
		},
	}}, nil
}

func (s *SessionAdminService) List(ctx context.Context, query SessionListQuery) (*SessionListResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = s.normalizeSessionListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}
	list := make([]SessionListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row, query.Now))
	}
	return &SessionListResponse{
		List: list,
		Page: SessionPage{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *SessionAdminService) Stats(ctx context.Context) (*SessionStatsResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
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
	return &SessionStatsResponse{TotalActive: total, PlatformDistribution: dist}, nil
}

func (s *SessionAdminService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*SessionRevokeResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
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
		return &SessionRevokeResponse{ID: id, Revoked: false}, nil
	}
	now := s.now()
	if _, err := repo.MarkRevoked(ctx, []int64{id}, now); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.revoke_failed", nil, "踢下线用户会话失败", err)
	}
	if err := s.revokeCache(ctx, *row); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.cache_revoke_failed", nil, "清理用户会话缓存失败", err)
	}
	return &SessionRevokeResponse{ID: id, Revoked: true}, nil
}

func (s *SessionAdminService) BatchRevoke(ctx context.Context, input SessionBatchRevokeInput, currentSessionID int64) (*SessionBatchRevokeResponse, *apperror.Error) {
	repo, appErr := s.requireSessionAdminRepository()
	if appErr != nil {
		return nil, appErr
	}
	ids := normalizeIDs(input.IDs)
	if len(ids) > 100 {
		return nil, apperror.BadRequestKey("usersession.batch_too_many", nil, "单次最多踢下线100个会话")
	}
	if len(ids) == 0 {
		return &SessionBatchRevokeResponse{}, nil
	}
	rows, err := repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "usersession.query_failed", nil, "查询用户会话失败", err)
	}

	response := &SessionBatchRevokeResponse{}
	toRevoke := make([]SessionAdminRecord, 0, len(rows))
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

func (s *SessionAdminService) requireSessionAdminRepository() (SessionAdminRepository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("usersession.repository_missing", nil, "用户会话仓储未配置")
	}
	return s.repository, nil
}

func (s *SessionAdminService) normalizeSessionListQuery(query SessionListQuery) (SessionListQuery, *apperror.Error) {
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

func (s *SessionAdminService) revokeCache(ctx context.Context, row SessionAdminRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	return s.cacheRevoker.RevokeCache(ctx, sessionFromAdminRecord(row))
}

func (s *SessionAdminService) revokeCaches(ctx context.Context, rows []SessionAdminRecord) error {
	if s == nil || s.cacheRevoker == nil {
		return nil
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessionFromAdminRecord(row))
	}
	return s.cacheRevoker.RevokeCaches(ctx, sessions)
}

func sessionFromAdminRecord(row SessionAdminRecord) Session {
	return Session{ID: row.ID, UserID: row.UserID, Platform: row.Platform, AccessTokenHash: row.AccessTokenHash}
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

func listItem(row SessionListRow, now time.Time) SessionListItem {
	return SessionListItem{
		ID: row.ID, UserID: row.UserID, Username: row.Username,
		Platform: row.Platform, PlatformName: platformName(row.Platform),
		DeviceID: row.DeviceID, IP: row.IP, UserAgent: row.UserAgent,
		LastSeenAt: formatTime(row.LastSeenAt), CreatedAt: formatTime(row.CreatedAt),
		ExpiresAt: formatTime(row.ExpiresAt), RefreshExpiresAt: formatTime(row.RefreshExpiresAt),
		RevokedAt: formatOptionalTime(row.RevokedAt), Status: sessionStatus(row, now),
	}
}

func sessionStatus(row SessionListRow, now time.Time) string {
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
	formatted := value.Format(sessionAdminTimeLayout)
	return &formatted
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(sessionAdminTimeLayout)
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
