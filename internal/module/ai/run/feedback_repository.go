package airun

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *GormRepository) FeedbackRun(ctx context.Context, id int64) (*FeedbackRun, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row FeedbackRun
	err := r.db.WithContext(ctx).Table("ai_runs").
		Select("id, user_id, conversation_id, status, assistant_message_id, liked_at").
		Where("id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateUserFeedback(ctx context.Context, id int64, userID int64, liked bool, now time.Time) (*time.Time, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	value := any(nil)
	if liked {
		value = gorm.Expr("COALESCE(liked_at, ?)", now)
	}
	if err := r.db.WithContext(ctx).Table("ai_runs").
		Where("id = ? AND user_id = ?", id, userID).
		Update("liked_at", value).Error; err != nil {
		return nil, false, err
	}
	var row struct{ LikedAt *time.Time }
	err := r.db.WithContext(ctx).Table("ai_runs").
		Select("liked_at").
		Where("id = ? AND user_id = ?", id, userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return row.LikedAt, true, nil
}
