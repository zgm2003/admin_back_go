package userloginlog

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
const dateLayout = "2006-01-02"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		PlatformArr:  dict.PlatformOptions(),
		LoginTypeArr: dict.AuthPlatformLoginTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("用户登录日志仓储未配置")
	}
	query, appErr := normalizeQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := s.repository.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询用户登录日志失败", err)
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

func normalizeQuery(query ListQuery) (ListQuery, *apperror.Error) {
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
		return query, apperror.BadRequest("无效的登录类型")
	}
	if query.Platform != "" && !enum.IsPlatform(query.Platform) {
		return query, apperror.BadRequest("无效的平台标识")
	}
	if query.IsSuccess != nil && !enum.IsCommonYesNo(*query.IsSuccess) {
		return query, apperror.BadRequest("无效的登录结果")
	}
	if query.DateStart != "" {
		if _, err := time.Parse(dateLayout, query.DateStart); err != nil {
			return query, apperror.BadRequest("无效的开始日期")
		}
		query.CreatedStart = query.DateStart + " 00:00:00"
	}
	if query.DateEnd != "" {
		if _, err := time.Parse(dateLayout, query.DateEnd); err != nil {
			return query, apperror.BadRequest("无效的结束日期")
		}
		query.CreatedEnd = query.DateEnd + " 23:59:59"
	}
	return query, nil
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, UserID: row.UserID, UserName: row.Username,
		LoginAccount: row.LoginAccount,
		LoginType:    row.LoginType, LoginTypeName: loginTypeName(row.LoginType),
		Platform: row.Platform, PlatformName: platformName(row.Platform),
		IP: row.IP, UserAgent: row.UserAgent, IsSuccess: row.IsSuccess,
		Reason: row.Reason, CreatedAt: formatTime(row.CreatedAt),
	}
}

func loginTypeName(value string) string {
	for _, item := range dict.AuthPlatformLoginTypeOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func platformName(value string) string {
	for _, item := range dict.PlatformOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return value
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
