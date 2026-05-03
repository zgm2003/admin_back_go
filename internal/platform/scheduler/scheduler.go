package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/config"

	gocron "github.com/go-co-op/gocron/v2"
)

var (
	ErrInvalidTimezone     = errors.New("scheduler timezone is invalid")
	ErrJobNameRequired     = errors.New("scheduler job name is required")
	ErrJobIntervalRequired = errors.New("scheduler job interval is required")
	ErrJobTaskRequired     = errors.New("scheduler job task is required")
)

type TaskFunc func(ctx context.Context) error

// Scheduler wraps gocron so jobs do not depend on gocron directly.
type Scheduler struct {
	scheduler gocron.Scheduler
	location  *time.Location
}

// New creates a scheduler using the configured timezone.
func New(cfg config.SchedulerConfig) (*Scheduler, error) {
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
	return &Scheduler{scheduler: s, location: location}, nil
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

	_, err := s.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(func(ctx context.Context) error {
			return task(ctx)
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

	_, err := s.scheduler.NewJob(
		gocron.CronJob(expression, withSeconds),
		gocron.NewTask(func(ctx context.Context) error {
			return task(ctx)
		}),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	return err
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
