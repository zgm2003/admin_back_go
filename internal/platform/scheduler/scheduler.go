package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/redislock"

	gocron "github.com/go-co-op/gocron/v2"
)

var (
	ErrInvalidTimezone     = errors.New("scheduler timezone is invalid")
	ErrJobNameRequired     = errors.New("scheduler job name is required")
	ErrJobIntervalRequired = errors.New("scheduler job interval is required")
	ErrJobTaskRequired     = errors.New("scheduler job task is required")
)

type TaskFunc func(ctx context.Context) error

type Locker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (string, error)
	Unlock(ctx context.Context, key string, token string) error
}

type Option func(*Scheduler)

func WithLocker(locker Locker) Option {
	return func(s *Scheduler) {
		s.locker = locker
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Scheduler) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// Scheduler wraps gocron so jobs do not depend on gocron directly.
type Scheduler struct {
	scheduler  gocron.Scheduler
	location   *time.Location
	lockPrefix string
	lockTTL    time.Duration
	locker     Locker
	logger     *slog.Logger
}

// New creates a scheduler using the configured timezone.
func New(cfg config.SchedulerConfig, opts ...Option) (*Scheduler, error) {
	timezone := strings.TrimSpace(cfg.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTimezone, timezone)
	}

	s, err := gocron.NewScheduler(gocron.WithLocation(location))
	if err != nil {
		return nil, err
	}
	result := &Scheduler{
		scheduler:  s,
		location:   location,
		lockPrefix: strings.TrimSpace(cfg.LockPrefix),
		lockTTL:    cfg.LockTTL,
		logger:     slog.Default(),
	}
	if result.lockTTL <= 0 {
		result.lockTTL = 30 * time.Second
	}
	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}
	return result, nil
}

// Every registers a non-overlapping interval job.
func (s *Scheduler) Every(name string, interval time.Duration, task TaskFunc) error {
	if strings.TrimSpace(name) == "" {
		return ErrJobNameRequired
	}
	if interval <= 0 {
		return ErrJobIntervalRequired
	}
	if task == nil {
		return ErrJobTaskRequired
	}
	wrappedTask := s.wrapTask(name, task)

	_, err := s.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(func(ctx context.Context) error {
			return wrappedTask(ctx)
		}),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	return err
}

// Cron registers a non-overlapping cron expression job.
func (s *Scheduler) Cron(name string, expression string, withSeconds bool, task TaskFunc) error {
	if strings.TrimSpace(name) == "" {
		return ErrJobNameRequired
	}
	if strings.TrimSpace(expression) == "" {
		return ErrJobIntervalRequired
	}
	if task == nil {
		return ErrJobTaskRequired
	}
	wrappedTask := s.wrapTask(name, task)

	_, err := s.scheduler.NewJob(
		gocron.CronJob(expression, withSeconds),
		gocron.NewTask(func(ctx context.Context) error {
			return wrappedTask(ctx)
		}),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	return err
}

func (s *Scheduler) wrapTask(name string, task TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		if s == nil || s.locker == nil || strings.TrimSpace(s.lockPrefix) == "" {
			return task(ctx)
		}
		key := s.lockPrefix + strings.TrimSpace(name)
		token, err := s.locker.Lock(ctx, key, s.lockTTL)
		if errors.Is(err, redislock.ErrNotAcquired) {
			if s.logger != nil {
				s.logger.InfoContext(ctx, "skip scheduler job because distributed lock is held", "name", name, "lock_key", key)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("scheduler lock %s: %w", name, err)
		}
		defer func() {
			if unlockErr := s.locker.Unlock(ctx, key, token); unlockErr != nil && s.logger != nil {
				s.logger.ErrorContext(ctx, "unlock scheduler job failed", "name", name, "lock_key", key, "error", unlockErr)
			}
		}()
		return task(ctx)
	}
}

// Start begins scheduling. It is non-blocking.
func (s *Scheduler) Start() {
	if s == nil || s.scheduler == nil {
		return
	}
	s.scheduler.Start()
}

// Shutdown stops scheduling and respects the provided context deadline.
func (s *Scheduler) Shutdown(ctx context.Context) error {
	if s == nil || s.scheduler == nil {
		return nil
	}
	if ctx == nil {
		return s.scheduler.Shutdown()
	}
	return s.scheduler.ShutdownWithContext(ctx)
}
