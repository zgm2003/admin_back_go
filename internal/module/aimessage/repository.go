package aimessage

import (
	"context"
	"errors"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aimessage repository not configured")

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) Message(ctx context.Context, id int64) (*Message, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Message
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]Message, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Where("conversation_id = ?", query.ConversationID).Where("is_del = ?", enum.CommonNo)
	if query.Role != nil {
		db = db.Where("role = ?", *query.Role)
	}
	var total int64
	if err := db.Model(&Message{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Message
	err := db.Order("id ASC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) UpdateContent(ctx context.Context, id int64, content string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Message{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Update("content", content).Error
}

func (r *GormRepository) DeleteAfterMessage(ctx context.Context, conversationID int64, messageID int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	res := r.db.WithContext(ctx).Model(&Message{}).
		Where("conversation_id = ?", conversationID).
		Where("id > ?", messageID).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes)
	return res.RowsAffected, res.Error
}

func (r *GormRepository) UpdateMeta(ctx context.Context, id int64, metaJSON *string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Message{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Update("meta_json", metaJSON).Error
}

func (r *GormRepository) DeleteMessages(ctx context.Context, ids []int64, userID int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	res := r.db.WithContext(ctx).Table("ai_messages as m").
		Joins("JOIN ai_conversations c ON c.id = m.conversation_id AND c.user_id = ? AND c.is_del = ?", userID, enum.CommonNo).
		Where("m.id IN ?", ids).
		Where("m.is_del = ?", enum.CommonNo).
		Update("m.is_del", enum.CommonYes)
	return res.RowsAffected, res.Error
}
