package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/shared/apperror"
)

const TypeCleanupExpiredV1 = "realtime:cleanup-expired:v1"

var ErrRetentionServiceNotConfigured = errors.New("realtime retention service is not configured")

type EventCleaner interface {
	CleanupExpired(context.Context, time.Time, int) (CleanupResult, error)
}

type JobService interface {
	CleanupExpired(context.Context) error
}

type RetentionService struct {
	cleaner EventCleaner
	now     func() time.Time
}

func NewRetentionService(cleaner EventCleaner) *RetentionService {
	return &RetentionService{cleaner: cleaner, now: time.Now}
}

func (s *RetentionService) CleanupExpired(ctx context.Context) error {
	if s == nil || s.cleaner == nil {
		return ErrRetentionServiceNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cutoff := time.Now().UTC()
	if s.now != nil {
		cutoff = s.now().UTC()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := s.cleaner.CleanupExpired(ctx, cutoff, DefaultCleanupBatchSize)
		if err != nil {
			return err
		}
		if result.Deleted < DefaultCleanupBatchSize {
			return nil
		}
	}
}

func NewCleanupExpiredTask() (taskqueue.Task, error) {
	payload, err := json.Marshal(struct{}{})
	if err != nil {
		return taskqueue.Task{}, fmt.Errorf("encode %s payload: %w", TypeCleanupExpiredV1, err)
	}
	return taskqueue.Task{Type: TypeCleanupExpiredV1, Payload: payload}, nil
}

func RegisterTaskDefinitions(registry *taskqueue.Registry, service JobService, logger *slog.Logger) error {
	if registry == nil {
		return taskqueue.ErrRegistryRequired
	}
	if logger == nil {
		logger = slog.Default()
	}
	return registry.Register(taskqueue.Definition{
		Type:      TypeCleanupExpiredV1,
		Queue:     taskqueue.QueueLow,
		Timeout:   time.Minute,
		MaxRetry:  3,
		UniqueTTL: 23 * time.Hour,
		Decode: func(data []byte) (any, *apperror.Error) {
			var payload struct{}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&payload); err != nil {
				return nil, taskqueue.PayloadError(TypeCleanupExpiredV1, err)
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				return nil, taskqueue.PayloadError(TypeCleanupExpiredV1, errors.New("trailing JSON content"))
			}
			return payload, nil
		},
		Handle: func(ctx context.Context, _ any) *apperror.Error {
			if service == nil {
				return taskqueue.InvariantError(TypeCleanupExpiredV1, ErrRetentionServiceNotConfigured)
			}
			if err := service.CleanupExpired(ctx); err != nil {
				logger.WarnContext(ctx, "realtime retention cleanup failed", "error", err)
				return taskqueue.HandlerError(TypeCleanupExpiredV1, err)
			}
			return nil
		},
	})
}

var _ JobService = (*RetentionService)(nil)
