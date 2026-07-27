package airun

import (
	"context"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

type FeedbackRun struct {
	ID                 int64
	UserID             int64
	ConversationID     *int64
	Status             string
	AssistantMessageID *int64
	LikedAt            *time.Time
}

type FeedbackResponse struct {
	ID      int64   `json:"id"`
	Liked   bool    `json:"liked"`
	LikedAt *string `json:"liked_at"`
}

type FeedbackRepository interface {
	FeedbackRun(ctx context.Context, id int64) (*FeedbackRun, error)
	UpdateUserFeedback(ctx context.Context, id int64, userID int64, liked bool, now time.Time) (*time.Time, bool, error)
}

func (s *Service) SetUserFeedback(ctx context.Context, userID int64, id int64, liked bool) (*FeedbackResponse, *apperror.Error) {
	if userID <= 0 {
		return nil, apperror.UnauthorizedKey("airun.feedback.unauthorized", nil, "Token无效或已过期")
	}
	if id <= 0 {
		return nil, apperror.BadRequestKey("airun.feedback.id_invalid", nil, "无效的AI运行ID")
	}
	repository, appErr := s.requireFeedbackRepository()
	if appErr != nil {
		return nil, appErr
	}
	run, err := repository.FeedbackRun(ctx, id)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "airun.feedback.query_failed", nil, "查询AI运行反馈状态失败", err)
	}
	if run == nil {
		return nil, apperror.NotFoundKey("airun.feedback.not_found", nil, "AI运行记录不存在")
	}
	if run.UserID != userID {
		return nil, apperror.NotFoundKey("airun.feedback.not_found", nil, "AI运行记录不存在")
	}
	if run.ConversationID == nil || *run.ConversationID <= 0 || run.Status != enum.AIRunStatusSuccess || run.AssistantMessageID == nil || *run.AssistantMessageID <= 0 {
		return nil, apperror.BadRequestKey("airun.feedback.unavailable", nil, "该AI运行不支持反馈")
	}
	if liked == (run.LikedAt != nil) {
		return feedbackResponse(id, run.LikedAt), nil
	}
	now := time.Time{}
	if liked {
		now = s.clock.Now()
	}
	likedAt, found, err := repository.UpdateUserFeedback(ctx, id, userID, liked, now)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "airun.feedback.update_failed", nil, "更新AI运行反馈失败", err)
	}
	if !found {
		return nil, apperror.NotFoundKey("airun.feedback.not_found", nil, "AI运行记录不存在")
	}
	return feedbackResponse(id, likedAt), nil
}

func (s *Service) requireFeedbackRepository() (FeedbackRepository, *apperror.Error) {
	if s == nil || s.feedbackRepository == nil {
		return nil, apperror.InternalKey("airun.feedback.repository_missing", nil, "AI运行反馈仓储未配置")
	}
	return s.feedbackRepository, nil
}

func feedbackResponse(id int64, likedAt *time.Time) *FeedbackResponse {
	var formatted *string
	if likedAt != nil {
		value := formatTime(*likedAt)
		formatted = &value
	}
	return &FeedbackResponse{ID: id, Liked: likedAt != nil, LikedAt: formatted}
}
