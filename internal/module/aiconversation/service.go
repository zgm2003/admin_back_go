package aiconversation

import (
	"context"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(userID, query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
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

func (s *Service) Detail(ctx context.Context, userID int64, id int64) (*DetailResponse, *apperror.Error) {
	row, appErr := s.requireOwnedConversation(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepository()
	appName, err := repo.AppName(ctx, row.AppID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI应用失败", err)
	}
	item := detailItem(*row, appName)
	return &item, nil
}

func (s *Service) Create(ctx context.Context, userID int64, input MutationInput) (int64, *apperror.Error) {
	if userID <= 0 {
		return 0, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := s.normalizeCreate(ctx, repo, userID, input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI会话失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, userID int64, id int64, input MutationInput) *apperror.Error {
	if _, appErr := s.requireOwnedConversation(ctx, userID, id); appErr != nil {
		return appErr
	}
	title, appErr := normalizeTitle(input.Title, true)
	if appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	if err := repo.Update(ctx, id, map[string]any{"title": title}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI会话失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, userID int64, id int64, status int) *apperror.Error {
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的会话状态")
	}
	if _, appErr := s.requireOwnedConversation(ctx, userID, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI会话状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	if _, appErr := s.requireOwnedConversation(ctx, userID, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI会话失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI会话仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireOwnedConversation(ctx context.Context, userID int64, id int64) (*Conversation, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequest("无效的AI会话ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI会话不存在")
	}
	if row.UserID != userID {
		return nil, apperror.Forbidden("无权访问该AI会话")
	}
	return row, nil
}

func (s *Service) normalizeCreate(ctx context.Context, repo Repository, userID int64, input MutationInput) (Conversation, *apperror.Error) {
	appID := effectiveAppID(input.AppID, input.AgentID)
	if appID <= 0 {
		return Conversation{}, apperror.BadRequest("AI应用ID不能为空")
	}
	ok, err := repo.ActiveAppExists(ctx, appID)
	if err != nil {
		return Conversation{}, apperror.Wrap(apperror.CodeInternal, 500, "校验AI应用失败", err)
	}
	if !ok {
		return Conversation{}, apperror.BadRequest("关联的AI应用不存在或已禁用")
	}
	title, appErr := normalizeTitle(input.Title, false)
	if appErr != nil {
		return Conversation{}, appErr
	}
	if title == "" {
		title = "新会话"
	}
	return Conversation{
		UserID: userID, AppID: appID, Title: title,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	}, nil
}

func effectiveAppID(appID int64, legacyAgentID int64) int64 {
	if appID > 0 {
		return appID
	}
	return legacyAgentID
}

func normalizeListQuery(userID int64, query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	if query.Status == nil {
		status := enum.CommonYes
		query.Status = &status
	}
	query.UserID = userID
	query.Title = strings.TrimSpace(query.Title)
	return query
}

func normalizeTitle(value string, required bool) (string, *apperror.Error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", apperror.BadRequest("会话标题不能为空")
	}
	if len([]rune(value)) > 100 {
		return "", apperror.BadRequest("会话标题不能超过100个字符")
	}
	return value, nil
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func listItem(row ListRow) ListItem {
	c := row.Conversation
	return ListItem{
		ID: c.ID, UserID: c.UserID,
		AppID: c.AppID, AppName: row.AppName,
		AgentID: c.AppID, AgentName: row.AppName,
		Title:         c.Title,
		LastMessageAt: formatOptionalTime(c.LastMessageAt), Status: c.Status, StatusName: statusName(c.Status),
		CreatedAt: formatTime(c.CreatedAt), UpdatedAt: formatTime(c.UpdatedAt),
	}
}

func detailItem(row Conversation, appName string) DetailResponse {
	return DetailResponse{
		ID: row.ID, UserID: row.UserID,
		AppID: row.AppID, AppName: appName,
		AgentID: row.AppID, AgentName: appName,
		Title:         row.Title,
		LastMessageAt: formatOptionalTime(row.LastMessageAt), Status: row.Status,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func statusName(value int) string {
	if value == enum.CommonYes {
		return "正常"
	}
	if value == enum.CommonNo {
		return "归档"
	}
	return ""
}
