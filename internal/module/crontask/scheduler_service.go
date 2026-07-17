package crontask

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/taskqueue"
)

type SchedulerService struct {
	repository Repository
	registry   Registry
	enqueuer   taskqueue.Enqueuer
	logger     *slog.Logger
	now        func() time.Time
}

func NewSchedulerService(repository Repository, registry Registry, enqueuer taskqueue.Enqueuer, logger *slog.Logger) *SchedulerService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SchedulerService{repository: repository, registry: registry, enqueuer: enqueuer, logger: logger, now: time.Now}
}

func (s *SchedulerService) taskFunc(row Task, entry RegistryEntry) scheduler.TaskFunc {
	return func(ctx context.Context) error {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		now := s.now()
		logID, err := s.repository.LogStart(ctx, row, now)
		if err != nil {
			return fmt.Errorf("start cron task log %s: %w", row.Name, err)
		}
		task, err := entry.BuildTask()
		if err != nil {
			_ = s.repository.LogEnd(ctx, logID, false, "", err.Error(), s.now())
			return fmt.Errorf("build cron task %s queue task: %w", row.Name, err)
		}
		if err := ctx.Err(); err != nil {
			_ = s.repository.LogEnd(context.WithoutCancel(ctx), logID, false, "", err.Error(), s.now())
			return err
		}
		if s.enqueuer == nil {
			err := taskqueue.ErrClientNotReady
			_ = s.repository.LogEnd(ctx, logID, false, "", err.Error(), s.now())
			return fmt.Errorf("enqueue cron task %s queue task %s: %w", row.Name, task.Type, err)
		}
		result, err := s.enqueuer.Enqueue(ctx, task)
		if err != nil {
			_ = s.repository.LogEnd(ctx, logID, false, "", err.Error(), s.now())
			return fmt.Errorf("enqueue cron task %s queue task %s: %w", row.Name, task.Type, err)
		}
		message := fmt.Sprintf("queued task_id=%s queue=%s type=%s", result.ID, result.Queue, result.Type)
		if err := s.repository.LogEnd(ctx, logID, true, message, "", s.now()); err != nil {
			return fmt.Errorf("finish cron task log %s: %w", row.Name, err)
		}
		s.logger.InfoContext(ctx, "cron task enqueued", "name", row.Name, "task_type", result.Type, "task_id", result.ID, "queue", result.Queue)
		return nil
	}
}
