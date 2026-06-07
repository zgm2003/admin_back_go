package aitext

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"

	"gorm.io/gorm"
)

const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

var ErrStoreNotConfigured = errors.New("aitext store not configured")

type Store interface {
	Create(ctx context.Context, input CreateInput) (uint64, error)
	Complete(ctx context.Context, input CompleteInput) error
	Fail(ctx context.Context, input FailInput) error
}

type TextTask struct {
	ID           uint64     `gorm:"column:id;primaryKey"`
	Platform     string     `gorm:"column:platform"`
	UserID       int64      `gorm:"column:user_id"`
	AgentID      uint64     `gorm:"column:agent_id"`
	ProviderID   uint64     `gorm:"column:provider_id"`
	ModelID      string     `gorm:"column:model_id"`
	Prompt       string     `gorm:"column:prompt"`
	Answer       *string    `gorm:"column:answer"`
	Status       string     `gorm:"column:status"`
	ErrorMessage *string    `gorm:"column:error_message"`
	StartedAt    *time.Time `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
	ElapsedMS    uint       `gorm:"column:elapsed_ms"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (TextTask) TableName() string { return "ai_text_tasks" }

type CreateInput struct {
	Platform   string
	UserID     int64
	AgentID    uint64
	ProviderID uint64
	ModelID    string
	Prompt     string
	Status     string
	StartedAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CompleteInput struct {
	ID         uint64
	Answer     string
	FinishedAt time.Time
	ElapsedMS  uint
}

type FailInput struct {
	ID           uint64
	ErrorMessage string
	FinishedAt   time.Time
	ElapsedMS    uint
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(client *database.Client) *GormStore {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormStore{db: client.Gorm}
}

func (s *GormStore) Create(ctx context.Context, input CreateInput) (uint64, error) {
	if s == nil || s.db == nil {
		return 0, ErrStoreNotConfigured
	}
	task := TextTask{
		Platform:   strings.TrimSpace(input.Platform),
		UserID:     input.UserID,
		AgentID:    input.AgentID,
		ProviderID: input.ProviderID,
		ModelID:    strings.TrimSpace(input.ModelID),
		Prompt:     input.Prompt,
		Status:     strings.TrimSpace(input.Status),
		StartedAt:  timePtr(input.StartedAt),
		CreatedAt:  input.CreatedAt,
		UpdatedAt:  input.UpdatedAt,
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

func (s *GormStore) Complete(ctx context.Context, input CompleteInput) error {
	if s == nil || s.db == nil {
		return ErrStoreNotConfigured
	}
	answer := input.Answer
	fields := map[string]any{
		"answer":        &answer,
		"status":        StatusSuccess,
		"error_message": nil,
		"finished_at":   input.FinishedAt,
		"elapsed_ms":    input.ElapsedMS,
		"updated_at":    input.FinishedAt,
	}
	return s.finish(ctx, input.ID, fields)
}

func (s *GormStore) Fail(ctx context.Context, input FailInput) error {
	if s == nil || s.db == nil {
		return ErrStoreNotConfigured
	}
	message := strings.TrimSpace(input.ErrorMessage)
	fields := map[string]any{
		"status":        StatusFailed,
		"error_message": &message,
		"finished_at":   input.FinishedAt,
		"elapsed_ms":    input.ElapsedMS,
		"updated_at":    input.FinishedAt,
	}
	return s.finish(ctx, input.ID, fields)
}

func (s *GormStore) finish(ctx context.Context, id uint64, fields map[string]any) error {
	if id == 0 {
		return gorm.ErrRecordNotFound
	}
	tx := s.db.WithContext(ctx).Model(&TextTask{}).Where("id = ? AND status = ?", id, StatusRunning).Updates(fields)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
