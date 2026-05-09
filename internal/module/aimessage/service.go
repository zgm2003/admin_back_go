package aimessage

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	defaultLimit = 20
	maxLimit     = 100
)

type Service struct {
	repository    Repository
	replyEnqueuer ReplyEnqueuer
}

type Option func(*Service)

func WithReplyEnqueuer(enqueuer ReplyEnqueuer) Option {
	return func(s *Service) { s.replyEnqueuer = enqueuer }
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
	if _, appErr := s.requireOwnedConversation(ctx, userID, query.ConversationID); appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepository()
	query.UserID = userID
	query = normalizeListQuery(query)
	rows, hasMore, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI消息失败", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	list := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, messageItem(row))
	}
	nextID := int64(0)
	if hasMore && len(rows) > 0 {
		nextID = rows[0].ID
	}
	return &ListResponse{List: list, NextID: nextID, HasMore: hasMore}, nil
}

func (s *Service) Send(ctx context.Context, userID int64, input SendInput) (*SendResponse, *apperror.Error) {
	conversation, appErr := s.requireOwnedConversation(ctx, userID, input.ConversationID)
	if appErr != nil {
		return nil, appErr
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil, apperror.BadRequest("request_id不能为空")
	}
	repo, _ := s.requireRepository()
	agent, err := repo.AgentForConversation(ctx, input.ConversationID, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if agent == nil || agent.Status != enum.CommonYes || !agentSupportsChat(agent.ScenesJSON) {
		return nil, apperror.BadRequest("该智能体不支持对话场景")
	}
	userMessageID, err := repo.InsertUserMessage(ctx, SendRecord{ConversationID: input.ConversationID, Role: enum.AIMessageRoleUser, ContentType: "text", Content: content})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "保存AI消息失败", err)
	}
	if s.replyEnqueuer == nil {
		return nil, apperror.Internal("AI对话回复队列未配置")
	}
	payload := ReplyPayload{ConversationID: conversation.ID, UserID: userID, AgentID: conversation.AgentID, UserMessageID: userMessageID, RequestID: requestID}
	if err := s.replyEnqueuer.EnqueueConversationReply(ctx, payload); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "AI对话回复任务入队失败", err)
	}
	return &SendResponse{ConversationID: input.ConversationID, UserMessageID: userMessageID, RequestID: requestID}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI消息仓储未配置")
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
	row, err := repo.Conversation(ctx, id)
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

func normalizeListQuery(query ListQuery) ListQuery {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}
	return query
}

func messageItem(row Message) MessageItem {
	contentType := strings.TrimSpace(row.ContentType)
	if contentType == "" {
		contentType = "text"
	}
	return MessageItem{ID: row.ID, Role: row.Role, ContentType: contentType, Content: row.Content, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func agentSupportsChat(raw string) bool {
	var scenes []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &scenes); err != nil || len(scenes) == 0 {
		return false
	}
	for _, scene := range scenes {
		if strings.TrimSpace(scene) == "chat" {
			return true
		}
	}
	return false
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
