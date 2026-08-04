package aiconversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	defaultLimit = 20
	maxLimit     = 100
)

type Service struct {
	repository      Repository
	cancelPublisher replycommand.CancelPublisher
}

type Option func(*Service)

func WithCancelPublisher(publisher replycommand.CancelPublisher) Option {
	return func(service *Service) { service.cancelPublisher = publisher }
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.UserID = userID
	query, appErr = normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, hasMore, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
	}
	conversationIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		conversationIDs = append(conversationIDs, row.Conversation.ID)
	}
	unreadCounts := make(map[int64]uint64)
	if len(conversationIDs) > 0 {
		unreadCounts, err = repo.UnreadCounts(ctx, conversationIDs)
		if err != nil {
			return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiconversation.unread_count_failed", nil, "查询AI会话未读数失败", err)
		}
	}
	list := make([]ConversationItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, conversationItem(row, unreadCounts[row.Conversation.ID]))
	}
	nextID := int64(0)
	nextTime := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1].Conversation
		nextID = last.ID
		nextTime = formatTimePtr(last.LastMessageAt)
	}
	return &ListResponse{List: list, NextTime: nextTime, NextID: nextID, HasMore: hasMore}, nil
}

func (s *Service) AdvanceReadCursor(ctx context.Context, userID int64, conversationID int64, messageID int64) (*ReadCursorResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if conversationID <= 0 {
		return nil, apperror.BadRequest("无效的AI会话ID")
	}
	if messageID <= 0 {
		return nil, apperror.BadRequestKey("aiconversation.read_cursor.message_id_invalid", nil, "无效的AI消息ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cursor, unreadCount, valid, err := repo.AdvanceReadCursor(ctx, conversationID, userID, messageID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, apperror.Wrap(
				"request.canceled",
				apperror.CategoryCanceled,
				0,
				apperror.Permanent,
				"",
				nil,
				"请求已取消",
				err,
			)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "aiconversation.read_cursor.update_failed", nil, "更新AI会话已读位置失败", err)
	}
	if !valid {
		return nil, apperror.NotFoundKey("aiconversation.read_cursor.message_invalid", nil, "AI消息不存在或不可用于已读位置")
	}
	return &ReadCursorResponse{ConversationID: conversationID, LastReadMessageID: cursor, UnreadCount: unreadCount}, nil
}

func (s *Service) Detail(ctx context.Context, userID int64, id int64) (*ConversationDetail, *apperror.Error) {
	row, agentName, appErr := s.requireOwnedConversation(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	return &ConversationDetail{ID: row.ID, AgentID: row.AgentID, AgentName: agentName, Title: row.Title, LastMessageAt: formatTimePtr(row.LastMessageAt), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}, nil
}

func (s *Service) Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error) {
	if userID <= 0 {
		return 0, apperror.Unauthorized("Token无效或已过期")
	}
	if input.AgentID <= 0 {
		return 0, apperror.BadRequest("AI智能体ID不能为空")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	ok, err := repo.ActiveChatAgentExists(ctx, input.AgentID)
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if !ok {
		return 0, apperror.BadRequest("该智能体不支持对话场景")
	}
	now := time.Now()
	id, err := repo.Create(ctx, Conversation{UserID: userID, AgentID: input.AgentID, Title: trimTitle(input.Title), LastMessageAt: &now, IsDel: enum.CommonNo})
	if err != nil {
		return 0, apperror.LegacyWrap(apperror.CodeInternal, 500, "创建AI会话失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error {
	if _, _, appErr := s.requireOwnedConversation(ctx, userID, id); appErr != nil {
		return appErr
	}
	title := trimTitle(input.Title)
	if title == "" {
		return apperror.BadRequest("AI会话标题不能为空")
	}
	repo, _ := s.requireRepository()
	if err := repo.UpdateTitle(ctx, id, userID, title); err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "更新AI会话失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	if _, _, appErr := s.requireOwnedConversation(ctx, userID, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	result, err := repo.Delete(ctx, id, userID)
	if err != nil {
		return apperror.LegacyWrap(apperror.CodeInternal, 500, "删除AI会话失败", err)
	}
	if s.cancelPublisher != nil {
		for _, commandID := range result.CanceledCommandIDs {
			_ = s.cancelPublisher.PublishCancel(context.WithoutCancel(ctx), commandID)
		}
	}
	return nil
}

func (s *Service) requireOwnedConversation(ctx context.Context, userID int64, id int64) (*Conversation, string, *apperror.Error) {
	if userID <= 0 {
		return nil, "", apperror.Unauthorized("Token无效或已过期")
	}
	if id <= 0 {
		return nil, "", apperror.BadRequest("无效的AI会话ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, "", appErr
	}
	row, agentName, err := repo.Get(ctx, id)
	if err != nil {
		return nil, "", apperror.LegacyWrap(apperror.CodeInternal, 500, "查询AI会话失败", err)
	}
	if row == nil {
		return nil, "", apperror.NotFound("AI会话不存在")
	}
	if row.UserID != userID {
		return nil, "", apperror.Forbidden("无权访问该AI会话")
	}
	return row, agentName, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI会话仓储未配置")
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}
	hasTime := query.BeforeTime != nil && !query.BeforeTime.IsZero()
	hasID := query.BeforeID > 0
	if hasTime != hasID {
		return query, apperror.BadRequestKey("aiconversation.cursor.invalid", nil, "AI会话游标必须同时包含before_time和before_id")
	}
	return query, nil
}

func conversationItem(row ListRow, unreadCount uint64) ConversationItem {
	conv := row.Conversation
	return ConversationItem{ID: conv.ID, AgentID: conv.AgentID, AgentName: row.AgentName, Title: conv.Title, UnreadCount: unreadCount, LastMessageAt: formatTimePtr(conv.LastMessageAt), UpdatedAt: formatTime(conv.UpdatedAt)}
}

func trimTitle(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= 100 {
		return value
	}
	return string([]rune(value)[:100])
}

func formatTimePtr(value *time.Time) string {
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
