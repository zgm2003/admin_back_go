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
	attachments, appErr := normalizeAttachments(input.Attachments)
	if appErr != nil {
		return nil, appErr
	}
	if content == "" && len(attachments) == 0 {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	runtimeParams, appErr := normalizeRuntimeParams(input.RuntimeParams)
	if appErr != nil {
		return nil, appErr
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
	userMessageID, err := repo.InsertUserMessage(ctx, SendRecord{ConversationID: input.ConversationID, Role: enum.AIMessageRoleUser, ContentType: "text", Content: content, MetaJSON: metaJSONForSend(attachments, runtimeParams)})
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

func (s *Service) Cancel(ctx context.Context, userID int64, input CancelInput) (*CancelResponse, *apperror.Error) {
	if _, appErr := s.requireOwnedConversation(ctx, userID, input.ConversationID); appErr != nil {
		return nil, appErr
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil, apperror.BadRequest("request_id不能为空")
	}
	canceler, ok := s.replyEnqueuer.(ReplyCanceler)
	if !ok || canceler == nil {
		return nil, apperror.Internal("AI对话取消器未配置")
	}
	if err := canceler.CancelConversationReply(ctx, ReplyPayload{ConversationID: input.ConversationID, UserID: userID, RequestID: requestID}); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "取消AI回复失败", err)
	}
	return &CancelResponse{ConversationID: input.ConversationID, RequestID: requestID, Status: "canceled"}, nil
}

func metaJSONForSend(attachments []Attachment, runtimeParams map[string]float64) *string {
	meta := map[string]any{}
	if len(attachments) > 0 {
		meta["attachments"] = attachments
	}
	if len(runtimeParams) > 0 {
		meta["runtime_params"] = runtimeParams
	}
	if len(meta) == 0 {
		return nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	value := string(data)
	return &value
}

func normalizeAttachments(input []Attachment) ([]Attachment, *apperror.Error) {
	if len(input) == 0 {
		return nil, nil
	}
	if len(input) > 5 {
		return nil, apperror.BadRequest("图片数量不能超过5张")
	}
	out := make([]Attachment, 0, len(input))
	for _, item := range input {
		typ := strings.TrimSpace(item.Type)
		url := strings.TrimSpace(item.URL)
		if typ != "image" {
			return nil, apperror.BadRequest("仅支持图片附件")
		}
		if url == "" {
			return nil, apperror.BadRequest("图片地址不能为空")
		}
		if item.Size < 0 {
			return nil, apperror.BadRequest("图片大小非法")
		}
		out = append(out, Attachment{Type: "image", URL: url, Name: strings.TrimSpace(item.Name), Size: item.Size})
	}
	return out, nil
}

func normalizeRuntimeParams(input map[string]float64) (map[string]float64, *apperror.Error) {
	if len(input) == 0 {
		return nil, nil
	}
	out := map[string]float64{}
	for key, value := range input {
		switch key {
		case "temperature":
			if value < 0 || value > 2 {
				return nil, apperror.BadRequest("temperature必须在0到2之间")
			}
			out[key] = value
		case "max_tokens":
			if value < 1 || value > 200000 || value != float64(int64(value)) {
				return nil, apperror.BadRequest("max_tokens必须是正整数")
			}
			out[key] = value
		case "max_history":
			if value < 1 || value > 50 || value != float64(int64(value)) {
				return nil, apperror.BadRequest("max_history必须是1到50之间的整数")
			}
			out[key] = value
		default:
			return nil, apperror.BadRequest("不支持的AI运行参数")
		}
	}
	return out, nil
}

func decodeMetaJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return decoded
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
	metaJSON := ""
	if row.MetaJSON != nil {
		metaJSON = *row.MetaJSON
	}
	return MessageItem{ID: row.ID, Role: row.Role, ContentType: contentType, Content: row.Content, MetaJSON: decodeMetaJSON(metaJSON), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
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
