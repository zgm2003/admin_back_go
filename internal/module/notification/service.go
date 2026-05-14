package notification

import (
	"context"
	"math"
	"sort"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		NotificationTypeArr:       dict.NotificationTypeOptions(),
		NotificationLevelArr:      dict.NotificationLevelOptions(),
		NotificationReadStatusArr: dict.NotificationReadStatusOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notification.query_failed", nil, "查询通知失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItemFromNotification(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) UnreadCount(ctx context.Context, identity Identity) (*UnreadCountResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return nil, appErr
	}
	count, err := repo.UnreadCount(ctx, identity.UserID, identity.Platform)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "notification.unread_count_failed", nil, "查询未读通知数量失败", err)
	}
	return &UnreadCountResponse{Count: count}, nil
}

func (s *Service) MarkRead(ctx context.Context, identity Identity, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	_, err := repo.MarkRead(ctx, MarkReadInput{UserID: identity.UserID, Platform: identity.Platform, IDs: normalizeIDs(ids)})
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notification.mark_read_failed", nil, "标记通知已读失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, identity Identity, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	identity, appErr = normalizeIdentity(identity)
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequestKey("notification.delete.empty", nil, "请选择要删除的通知")
	}
	_, err := repo.Delete(ctx, DeleteInput{UserID: identity.UserID, Platform: identity.Platform, IDs: ids})
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "notification.delete_failed", nil, "删除通知失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("notification.repository_missing", nil, "通知仓储未配置")
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	identity, appErr := normalizeIdentity(Identity{UserID: query.UserID, Platform: query.Platform})
	if appErr != nil {
		return query, appErr
	}
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequestKey("notification.current_page.invalid", nil, "当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return query, apperror.BadRequestKey("notification.page_size.invalid", nil, "每页数量无效")
	}
	if query.Type != nil && !enum.IsNotificationType(*query.Type) {
		return query, apperror.BadRequestKey("notification.type.invalid", nil, "无效的通知类型")
	}
	if query.Level != nil && !enum.IsNotificationLevel(*query.Level) {
		return query, apperror.BadRequestKey("notification.level.invalid", nil, "无效的通知级别")
	}
	if query.IsRead != nil && !enum.IsCommonYesNo(*query.IsRead) {
		return query, apperror.BadRequestKey("notification.read_status.invalid", nil, "无效的已读状态")
	}
	query.UserID = identity.UserID
	query.Platform = identity.Platform
	query.Keyword = strings.TrimSpace(query.Keyword)
	return query, nil
}

func normalizeIdentity(identity Identity) (Identity, *apperror.Error) {
	if identity.UserID <= 0 {
		return identity, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	identity.Platform = strings.TrimSpace(identity.Platform)
	if identity.Platform == "" {
		identity.Platform = enum.PlatformAdmin
	}
	return identity, nil
}

func normalizeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
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
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func listItemFromNotification(row Notification) ListItem {
	return ListItem{
		ID:        row.ID,
		Title:     row.Title,
		Content:   row.Content,
		Type:      row.Type,
		TypeText:  enum.NotificationTypeLabels[row.Type],
		Level:     row.Level,
		LevelText: enum.NotificationLevelLabels[row.Level],
		Link:      row.Link,
		IsRead:    row.IsRead,
		CreatedAt: formatTime(row.CreatedAt),
	}
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
