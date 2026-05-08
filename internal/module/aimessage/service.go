package aimessage

import (
	"context"
	"encoding/json"
	"math"
	"sort"
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
	if _, appErr := s.requireOwnedConversation(ctx, userID, query.ConversationID); appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	repo, _ := s.requireRepository()
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI消息失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, messageItem(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) EditContent(ctx context.Context, userID int64, id int64, content string) (*EditContentResponse, *apperror.Error) {
	msg, appErr := s.requireOwnedMessage(ctx, userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if msg.Role != enum.AIMessageRoleUser {
		return nil, apperror.BadRequest("只能编辑用户消息")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, apperror.BadRequest("消息内容不能为空")
	}
	repo, _ := s.requireRepository()
	if err := repo.UpdateContent(ctx, id, content); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "编辑AI消息失败", err)
	}
	count, err := repo.DeleteAfterMessage(ctx, msg.ConversationID, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "删除后续AI消息失败", err)
	}
	return &EditContentResponse{DeletedCount: count}, nil
}

func (s *Service) Feedback(ctx context.Context, userID int64, id int64, feedback *int) *apperror.Error {
	msg, appErr := s.requireOwnedMessage(ctx, userID, id)
	if appErr != nil {
		return appErr
	}
	meta := decodeObject(msg.MetaJSON)
	if feedback == nil {
		delete(meta, "feedback")
	} else {
		if *feedback != 1 && *feedback != 2 {
			return apperror.BadRequest("无效的反馈值")
		}
		meta["feedback"] = *feedback
	}
	raw := encodeObject(meta)
	repo, _ := s.requireRepository()
	if err := repo.UpdateMeta(ctx, id, &raw); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "保存AI消息反馈失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID int64, ids []int64) (*DeleteResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil, apperror.BadRequest("消息ID不能为空")
	}
	if len(ids) > 100 {
		return nil, apperror.BadRequest("单次最多删除100条消息")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	count, err := repo.DeleteMessages(ctx, ids, userID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "删除AI消息失败", err)
	}
	return &DeleteResponse{Affected: count}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI消息仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireOwnedMessage(ctx context.Context, userID int64, id int64) (*Message, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的AI消息ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	msg, err := repo.Message(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI消息失败", err)
	}
	if msg == nil {
		return nil, apperror.NotFound("AI消息不存在")
	}
	if _, appErr := s.requireOwnedConversation(ctx, userID, msg.ConversationID); appErr != nil {
		return nil, appErr
	}
	return msg, nil
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
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	return query
}

func normalizeIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func messageItem(row Message) ListItem {
	return ListItem{ID: row.ID, ConversationID: row.ConversationID, Role: row.Role, Content: row.Content, MetaJSON: decodeObject(row.MetaJSON), CreatedAt: formatTime(row.CreatedAt)}
}

func decodeObject(raw *string) JSONObject {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return JSONObject{}
	}
	var out JSONObject
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return JSONObject{}
	}
	return out
}

func encodeObject(value JSONObject) string {
	if value == nil {
		value = JSONObject{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
