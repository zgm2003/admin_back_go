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
)

const loginLogTimeLayout = "2006-01-02 15:04:05"
const loginLogDateLayout = "2006-01-02"

// LoginLog is the users_login_log table model owned by auth.
type LoginLog struct {
	ID           int64     `gorm:"column:id"`
	UserID       *int64    `gorm:"column:user_id"`
	LoginAccount string    `gorm:"column:login_account"`
	LoginType    string    `gorm:"column:login_type"`
	Platform     string    `gorm:"column:platform"`
	IP           string    `gorm:"column:ip"`
	UserAgent    string    `gorm:"column:ua"`
	IsSuccess    int       `gorm:"column:is_success"`
	Reason       string    `gorm:"column:reason"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (LoginLog) TableName() string {
	return "users_login_log"
}

type LoginLogHTTPService interface {
	PageInit(ctx context.Context) (*LoginLogPageInitResponse, *apperror.Error)
	List(ctx context.Context, query LoginLogListQuery) (*LoginLogListResponse, *apperror.Error)
}

type LoginLogPageInitResponse struct {
	Dict LoginLogPageInitDict `json:"dict"`
}

type LoginLogPageInitDict struct {
	PlatformArr  []dict.Option[string] `json:"platformArr"`
	LoginTypeArr []dict.Option[string] `json:"login_type_arr"`
}

type LoginLogListQuery struct {
	CurrentPage  int
	PageSize     int
	UserID       int64
	LoginAccount string
	LoginType    string
	IP           string
	Platform     string
	IsSuccess    *int
	DateStart    string
	DateEnd      string
	CreatedStart string
	CreatedEnd   string
}

type LoginLogPage struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type LoginLogListResponse struct {
	List []LoginLogListItem `json:"list"`
	Page LoginLogPage       `json:"page"`
}

type LoginLogListItem struct {
	ID            int64  `json:"id"`
	UserID        *int64 `json:"user_id"`
	UserName      string `json:"user_name"`
	LoginAccount  string `json:"login_account"`
	LoginType     string `json:"login_type"`
	LoginTypeName string `json:"login_type_name"`
	Platform      string `json:"platform"`
	PlatformName  string `json:"platform_name"`
	IP            string `json:"ip"`
	UserAgent     string `json:"ua"`
	IsSuccess     int    `json:"is_success"`
	Reason        string `json:"reason"`
	CreatedAt     string `json:"created_at"`
}

type LoginLogListRow struct {
	ID           int64
	UserID       *int64
	Username     string
	LoginAccount string
	LoginType    string
	Platform     string
	IP           string
	UserAgent    string
	IsSuccess    int
	Reason       string
	CreatedAt    time.Time
}

var ErrLoginLogRepositoryNotConfigured = errors.New("user login log repository is not configured")

type LoginLogRepository interface {
	List(ctx context.Context, query LoginLogListQuery) ([]LoginLogListRow, int64, error)
}

type LoginLogGormRepository struct {
	db *gorm.DB
}

func NewLoginLogGormRepository(client *database.Client) *LoginLogGormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &LoginLogGormRepository{db: client.Gorm}
}

func (r *LoginLogGormRepository) List(ctx context.Context, query LoginLogListQuery) ([]LoginLogListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrLoginLogRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).
		Table("users_login_log AS l").
		Joins("LEFT JOIN users AS u ON u.id = l.user_id AND u.is_del = ?", enum.CommonNo).
		Where("l.is_del = ?", enum.CommonNo)

	if query.UserID > 0 {
		db = db.Where("l.user_id = ?", query.UserID)
	}
	if query.LoginAccount != "" {
		db = db.Where("l.login_account LIKE ?", strings.TrimSpace(query.LoginAccount)+"%")
	}
	if query.LoginType != "" {
		db = db.Where("l.login_type = ?", query.LoginType)
	}
	if query.IP != "" {
		db = db.Where("l.ip LIKE ?", strings.TrimSpace(query.IP)+"%")
	}
	if query.Platform != "" {
		db = db.Where("l.platform = ?", query.Platform)
	}
	if query.IsSuccess != nil {
		db = db.Where("l.is_success = ?", *query.IsSuccess)
	}
	if query.CreatedStart != "" {
		db = db.Where("l.created_at >= ?", query.CreatedStart)
	}
	if query.CreatedEnd != "" {
		db = db.Where("l.created_at <= ?", query.CreatedEnd)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []LoginLogListRow
	err := db.Select(`
			l.id,
			l.user_id,
			COALESCE(u.username, '') AS username,
			l.login_account,
			l.login_type,
			l.platform,
			l.ip,
			COALESCE(l.ua, '') AS user_agent,
			l.is_success,
			COALESCE(l.reason, '') AS reason,
			l.created_at
		`).
		Order("l.id DESC").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

type LoginLogService struct {
	repository LoginLogRepository
}

func NewLoginLogService(repository LoginLogRepository) *LoginLogService {
	return &LoginLogService{repository: repository}
}

func (s *LoginLogService) PageInit(ctx context.Context) (*LoginLogPageInitResponse, *apperror.Error) {
	return &LoginLogPageInitResponse{Dict: LoginLogPageInitDict{
		PlatformArr:  dict.PlatformOptions(),
		LoginTypeArr: dict.AuthPlatformLoginTypeOptions(),
	}}, nil
}

func (s *LoginLogService) List(ctx context.Context, query LoginLogListQuery) (*LoginLogListResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("userloginlog.repository_missing", nil, "用户登录日志仓储未配置")
	}
	query, appErr := normalizeLoginLogQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := s.repository.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "userloginlog.query_failed", nil, "查询用户登录日志失败", err)
	}
	list := make([]LoginLogListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, loginLogListItem(row))
	}
	return &LoginLogListResponse{
		List: list,
		Page: LoginLogPage{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalLoginLogPages(total, query.PageSize), Total: total},
	}, nil
}

func normalizeLoginLogQuery(query LoginLogListQuery) (LoginLogListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.LoginAccount = strings.TrimSpace(query.LoginAccount)
	query.LoginType = strings.TrimSpace(query.LoginType)
	query.IP = strings.TrimSpace(query.IP)
	query.Platform = strings.TrimSpace(query.Platform)
	query.DateStart = strings.TrimSpace(query.DateStart)
	query.DateEnd = strings.TrimSpace(query.DateEnd)

	if query.LoginType != "" && !enum.IsLoginType(query.LoginType) {
		return query, apperror.BadRequestKey("userloginlog.login_type.invalid", nil, "无效的登录类型")
	}
	if query.Platform != "" && !enum.IsPlatform(query.Platform) {
		return query, apperror.BadRequestKey("userloginlog.platform.invalid", nil, "无效的平台标识")
	}
	if query.IsSuccess != nil && !enum.IsCommonYesNo(*query.IsSuccess) {
		return query, apperror.BadRequestKey("userloginlog.result.invalid", nil, "无效的登录结果")
	}
	if query.DateStart != "" {
		if _, err := time.Parse(loginLogDateLayout, query.DateStart); err != nil {
			return query, apperror.BadRequestKey("userloginlog.date_start.invalid", nil, "无效的开始日期")
		}
		query.CreatedStart = query.DateStart + " 00:00:00"
	}
	if query.DateEnd != "" {
		if _, err := time.Parse(loginLogDateLayout, query.DateEnd); err != nil {
			return query, apperror.BadRequestKey("userloginlog.date_end.invalid", nil, "无效的结束日期")
		}
		query.CreatedEnd = query.DateEnd + " 23:59:59"
	}
	return query, nil
}

func loginLogListItem(row LoginLogListRow) LoginLogListItem {
	return LoginLogListItem{
		ID: row.ID, UserID: row.UserID, UserName: row.Username,
		LoginAccount: row.LoginAccount,
		LoginType:    row.LoginType, LoginTypeName: loginLogTypeName(row.LoginType),
		Platform: row.Platform, PlatformName: loginLogPlatformName(row.Platform),
		IP: row.IP, UserAgent: row.UserAgent, IsSuccess: row.IsSuccess,
		Reason: row.Reason, CreatedAt: formatLoginLogTime(row.CreatedAt),
	}
}

func loginLogTypeName(value string) string {
	for _, item := range dict.AuthPlatformLoginTypeOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func loginLogPlatformName(value string) string {
	for _, item := range dict.PlatformOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return value
}

func formatLoginLogTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(loginLogTimeLayout)
}

func totalLoginLogPages(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
