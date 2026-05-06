package crontask

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/platform/scheduler"
	"admin_back_go/internal/platform/taskqueue"
)

type ScheduleRegistrar interface {
	Every(name string, interval time.Duration, task scheduler.TaskFunc) error
	Cron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error
}

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

func (s *SchedulerService) RegisterEnabled(ctx context.Context, registrar ScheduleRegistrar) error {
	if registrar == nil {
		return scheduler.ErrJobTaskRequired
	}
	if s == nil || s.repository == nil {
		return ErrRepositoryNotConfigured
	}
	rows, err := s.repository.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("list enabled cron tasks: %w", err)
	}
	for _, row := range rows {
		entry, ok := s.registry.Lookup(row.Name)
		if !ok {
			s.logger.WarnContext(ctx, "skip unregistered cron task", "name", row.Name, "handler", row.Handler)
			continue
		}
		if !isValidCronExpression(row.Cron) {
			s.logger.WarnContext(ctx, "skip invalid cron task", "name", row.Name, "cron", row.Cron)
			continue
		}
		withSeconds := len(strings.Fields(row.Cron)) == 6
		if err := registrar.Cron(row.Name, row.Cron, withSeconds, s.taskFunc(row, entry)); err != nil {
			s.logger.ErrorContext(ctx, "register cron task failed", "name", row.Name, "cron", row.Cron, "error", err)
			continue
		}
		s.logger.InfoContext(ctx, "registered db-backed cron task", "name", row.Name, "cron", row.Cron, "task_type", entry.TaskType)
	}
	return nil
}

func (s *SchedulerService) taskFunc(row Task, entry RegistryEntry) scheduler.TaskFunc {
	return func(ctx context.Context) error {
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
