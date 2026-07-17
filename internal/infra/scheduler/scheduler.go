package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/redislock"
	"admin_back_go/internal/telemetry"

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

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(s *Scheduler) {
		if recorder != nil {
			s.recorder = recorder
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
	recorder   telemetry.Recorder
}

// New creates a scheduler using the configured timezone.
func New(cfg config.SchedulerConfig, opts ...Option) (*Scheduler, error) {
	cfg = config.NormalizeSchedulerConfig(cfg)
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTimezone, cfg.Timezone)
	}

	s, err := gocron.NewScheduler(gocron.WithLocation(location))
	if err != nil {
		return nil, err
	}
	result := &Scheduler{
		scheduler:  s,
		location:   location,
		lockPrefix: cfg.LockPrefix,
		lockTTL:    cfg.LockTTL,
		logger:     slog.Default(),
		recorder:   telemetry.Noop(),
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
		startedAt := time.Now()
		leaseOwned := true
		if s == nil || s.locker == nil || strings.TrimSpace(s.lockPrefix) == "" {
			err := task(ctx)
			s.recordExecution(time.Since(startedAt), leaseOwned, schedulerOutcome(err))
			return err
		}
		key := s.lockPrefix + strings.TrimSpace(name)
		token, err := s.locker.Lock(ctx, key, s.lockTTL)
		if errors.Is(err, redislock.ErrNotAcquired) {
			leaseOwned = false
			if s.logger != nil {
				s.logger.InfoContext(ctx, "skip scheduler job because distributed lock is held", "name", name, "lock_key", key)
			}
			s.recordExecution(time.Since(startedAt), leaseOwned, "skipped")
			return nil
		}
		if err != nil {
			leaseOwned = false
			s.recordExecution(time.Since(startedAt), leaseOwned, "error")
			return fmt.Errorf("scheduler lock %s: %w", name, err)
		}
		defer func() {
			if unlockErr := s.locker.Unlock(ctx, key, token); unlockErr != nil && s.logger != nil {
				s.logger.ErrorContext(ctx, "unlock scheduler job failed", "name", name, "lock_key", key, "error", unlockErr)
			}
		}()
		err = task(ctx)
		s.recordExecution(time.Since(startedAt), leaseOwned, schedulerOutcome(err))
		return err
	}
}

func (s *Scheduler) recordExecution(duration time.Duration, leaseOwned bool, outcome string) {
	recorder := s.recorder
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	attributes := telemetry.Attributes{
		"scheduler.operation":   "run",
		"scheduler.lease_owned": leaseOwned,
		"scheduler.outcome":     outcome,
	}
	recorder.Count("scheduler.executions", 1, attributes)
	recorder.Observe("scheduler.execution.duration_seconds", duration.Seconds(), attributes)
}

func schedulerOutcome(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
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
