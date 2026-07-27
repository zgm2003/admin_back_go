package aimessage

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/apperror"
)

const ErrorCodeHistoryActive = "ai.message.history_active"

var (
	ErrHistoryActiveCommand  = errors.New("active reply command blocks message history mutation")
	ErrHistorySourceNotFound = errors.New("message history source not found")
	ErrHistoryIDsInvalid     = errors.New("message history ids are invalid")
)

func (s *Service) Revise(ctx context.Context, userID int64, input EditInput) (*SendResponse, *apperror.Error) {
	if appErr := validateHistoryIdentity(userID, input.ConversationID, input.MessageID); appErr != nil {
		return nil, appErr
	}
	input.UserID = userID
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" {
		return nil, apperror.BadRequestKey("aimessage.revision.content.required", nil, "编辑文字不能为空")
	}
	if utf8.RuneCountInString(input.Content) > 20000 {
		return nil, apperror.BadRequestKey("aimessage.revision.content.too_long", nil, "编辑文字不能超过20000个字符")
	}
	requestID, appErr := normalizeHistoryRequestID(input.RequestID)
	if appErr != nil {
		return nil, appErr
	}
	input.RequestID = requestID
	history, appErr := s.requireHistoryRepository()
	if appErr != nil {
		return nil, appErr
	}
	result, err := history.Revise(ctx, input)
	if err != nil {
		return nil, historyActionError("aimessage.revision.failed", "编辑AI消息失败", err)
	}
	return s.acceptHistoryResult(ctx, input.ConversationID, result)
}

func (s *Service) Regenerate(ctx context.Context, userID int64, input RegenerateInput) (*SendResponse, *apperror.Error) {
	if appErr := validateHistoryIdentity(userID, input.ConversationID, input.AssistantMessageID); appErr != nil {
		return nil, appErr
	}
	input.UserID = userID
	requestID, appErr := normalizeHistoryRequestID(input.RequestID)
	if appErr != nil {
		return nil, appErr
	}
	input.RequestID = requestID
	history, appErr := s.requireHistoryRepository()
	if appErr != nil {
		return nil, appErr
	}
	result, err := history.Regenerate(ctx, input)
	if err != nil {
		return nil, historyActionError("aimessage.regeneration.failed", "重新生成AI回复失败", err)
	}
	return s.acceptHistoryResult(ctx, input.ConversationID, result)
}

func (s *Service) DeleteMessages(ctx context.Context, userID int64, input DeleteInput) (*DeleteResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("common.unauthorized", nil, "Token无效或已过期")
	}
	if input.ConversationID <= 0 {
		return nil, apperror.BadRequestKey("aimessage.conversation.id.invalid", nil, "无效的AI会话ID")
	}
	ids, appErr := normalizeDeleteIDs(input.IDs)
	if appErr != nil {
		return nil, appErr
	}
	input.UserID, input.IDs = userID, ids
	history, appErr := s.requireHistoryRepository()
	if appErr != nil {
		return nil, appErr
	}
	deleted, err := history.DeleteMessages(ctx, input)
	if err != nil {
		return nil, historyActionError("aimessage.delete.failed", "删除AI消息失败", err)
	}
	deleted = append([]int64(nil), deleted...)
	sort.Slice(deleted, func(i, j int) bool { return deleted[i] < deleted[j] })
	return &DeleteResponse{DeletedIDs: deleted}, nil
}

func (s *Service) acceptHistoryResult(ctx context.Context, conversationID int64, accepted HistoryAccepted) (*SendResponse, *apperror.Error) {
	result := accepted.Reply
	if result.UserMessageID <= 0 || result.CommandID == 0 || strings.TrimSpace(result.RequestID) == "" {
		return nil, apperror.InternalKey("aimessage.history.accept.invalid", nil, "AI消息历史操作接受结果无效")
	}
	if s.replyWaker != nil && !accepted.Replayed {
		if ctx == nil {
			ctx = context.Background()
		}
		_ = s.replyWaker.WakeReply(context.WithoutCancel(ctx), result.CommandID)
	}
	return &SendResponse{
		ConversationID: conversationID, UserMessageID: result.UserMessageID, CommandID: result.CommandID,
		RequestID: result.RequestID, State: result.State,
	}, nil
}

func validateHistoryIdentity(userID, conversationID, messageID int64) *apperror.Error {
	if userID <= 0 {
		return apperror.UnauthorizedKey("common.unauthorized", nil, "Token无效或已过期")
	}
	if conversationID <= 0 {
		return apperror.BadRequestKey("aimessage.conversation.id.invalid", nil, "无效的AI会话ID")
	}
	if messageID <= 0 {
		return apperror.BadRequestKey("aimessage.message.id.invalid", nil, "无效的AI消息ID")
	}
	return nil
}

func normalizeHistoryRequestID(value string) (string, *apperror.Error) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 128 {
		return "", apperror.BadRequestKey("aimessage.request_id.invalid", nil, "request_id不能为空且不能超过128个字符")
	}
	return value, nil
}

func normalizeDeleteIDs(ids []int64) ([]int64, *apperror.Error) {
	if len(ids) == 0 {
		return nil, apperror.BadRequestKey("aimessage.delete.ids.required", nil, "待删除消息不能为空")
	}
	seen := make(map[int64]struct{}, len(ids))
	out := append([]int64(nil), ids...)
	for _, id := range out {
		if id <= 0 {
			return nil, apperror.BadRequestKey("aimessage.delete.ids.invalid", nil, "待删除消息ID必须是正整数")
		}
		if _, ok := seen[id]; ok {
			return nil, apperror.BadRequestKey("aimessage.delete.ids.duplicate", nil, "待删除消息ID不能重复")
		}
		seen[id] = struct{}{}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func historyActionError(messageID, fallback string, err error) *apperror.Error {
	switch {
	case errors.Is(err, requestidentity.ErrRequestIdentityConflict), errors.Is(err, requestidentity.ErrRequestIdentityNotReplayable):
		return apperror.Wrap(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "aimessage.request_id.conflict", nil, "request_id与原请求内容冲突", err)
	case errors.Is(err, ErrHistoryActiveCommand):
		return apperror.Wrap(ErrorCodeHistoryActive, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "aimessage.history.active", nil, "存在进行中的AI回复，请先停止并等待完成", err)
	case errors.Is(err, ErrHistorySourceNotFound):
		return apperror.Wrap("resource.not_found", apperror.CategoryNotFound, http.StatusNotFound, apperror.Permanent, "aimessage.message.not_found", nil, "AI消息不存在", err)
	case errors.Is(err, ErrHistoryIDsInvalid):
		return apperror.Wrap("request.invalid", apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "aimessage.delete.ids.invalid", nil, "待删除消息无效", err)
	default:
		return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, messageID, nil, fallback, err)
	}
}

func (s *Service) requireHistoryRepository() (HistoryRepository, *apperror.Error) {
	if s == nil || s.history == nil {
		return nil, apperror.InternalKey("aimessage.history.repository_missing", nil, "AI消息历史仓储未配置")
	}
	return s.history, nil
}
