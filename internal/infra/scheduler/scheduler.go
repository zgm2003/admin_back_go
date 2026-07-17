package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
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
	ErrJobAlreadyExists    = errors.New("scheduler job already exists")
	ErrJobNotFound         = errors.New("scheduler job not found")
)

type TaskFunc func(ctx context.Context) error

type Job struct {
	Name        string
	Expression  string
	WithSeconds bool
}

type Option func(*Scheduler)

func WithLeaseStore(store redislock.LeaseStore) Option {
	return func(s *Scheduler) {
		s.leaseStore = store
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

type registeredJob struct {
	job  gocron.Job
	spec Job
}

// Scheduler wraps gocron and owns distributed execution leases.
type Scheduler struct {
	mu         sync.RWMutex
	scheduler  gocron.Scheduler
	jobs       map[string]registeredJob
	location   *time.Location
	lockPrefix string
	lockTTL    time.Duration
	leaseOwner string
	leaseStore redislock.LeaseStore
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
	owner, err := newLeaseOwner()
	if err != nil {
		return nil, err
	}
	gocronScheduler, err := gocron.NewScheduler(gocron.WithLocation(location))
	if err != nil {
		return nil, err
	}
	result := &Scheduler{
		scheduler:  gocronScheduler,
		jobs:       make(map[string]registeredJob),
		location:   location,
		lockPrefix: cfg.LockPrefix,
		lockTTL:    cfg.LockTTL,
		leaseOwner: owner,
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

// Every registers a named non-overlapping interval job.
func (s *Scheduler) Every(name string, interval time.Duration, task TaskFunc) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrJobNameRequired
	}
	if interval <= 0 {
		return ErrJobIntervalRequired
	}
	if task == nil {
		return ErrJobTaskRequired
	}
	if s == nil || s.scheduler == nil {
		return errors.New("scheduler is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[name]; exists {
		return fmt.Errorf("%w: %s", ErrJobAlreadyExists, name)
	}
	job, err := s.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(func(ctx context.Context) error { return s.wrapTask(name, task)(ctx) }),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return err
	}
	s.jobs[name] = registeredJob{job: job, spec: Job{Name: name, Expression: interval.String()}}
	return nil
}

// Cron registers a new cron job.
func (s *Scheduler) Cron(name string, expression string, withSeconds bool, task TaskFunc) error {
	return s.AddCron(name, expression, withSeconds, task)
}

func (s *Scheduler) AddCron(name string, expression string, withSeconds bool, task TaskFunc) error {
	name, expression, err := validateCronDefinition(name, expression, task)
	if err != nil {
		return err
	}
	if s == nil || s.scheduler == nil {
		return errors.New("scheduler is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[name]; exists {
		return fmt.Errorf("%w: %s", ErrJobAlreadyExists, name)
	}
	job, err := s.newCronJob(name, expression, withSeconds, task)
	if err != nil {
		return err
	}
	s.jobs[name] = registeredJob{job: job, spec: Job{Name: name, Expression: expression, WithSeconds: withSeconds}}
	return nil
}

func (s *Scheduler) UpdateCron(name string, expression string, withSeconds bool, task TaskFunc) error {
	name, expression, err := validateCronDefinition(name, expression, task)
	if err != nil {
		return err
	}
	if s == nil || s.scheduler == nil {
		return errors.New("scheduler is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.jobs[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, name)
	}
	job, err := s.scheduler.Update(
		current.job.ID(),
		gocron.CronJob(expression, withSeconds),
		gocron.NewTask(func(ctx context.Context) error { return s.wrapTask(name, task)(ctx) }),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return err
	}
	s.jobs[name] = registeredJob{job: job, spec: Job{Name: name, Expression: expression, WithSeconds: withSeconds}}
	return nil
}

func (s *Scheduler) Remove(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrJobNameRequired
	}
	if s == nil || s.scheduler == nil {
		return errors.New("scheduler is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.jobs[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, name)
	}
	if err := s.scheduler.RemoveJob(current.job.ID()); err != nil {
		return err
	}
	delete(s.jobs, name)
	return nil
}

func (s *Scheduler) List() []Job {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	jobs := make([]Job, 0, len(s.jobs))
	for _, registered := range s.jobs {
		jobs = append(jobs, registered.spec)
	}
	s.mu.RUnlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })
	return jobs
}

func (s *Scheduler) newCronJob(name string, expression string, withSeconds bool, task TaskFunc) (gocron.Job, error) {
	return s.scheduler.NewJob(
		gocron.CronJob(expression, withSeconds),
		gocron.NewTask(func(ctx context.Context) error { return s.wrapTask(name, task)(ctx) }),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
}

func validateCronDefinition(name string, expression string, task TaskFunc) (string, string, error) {
	name = strings.TrimSpace(name)
	expression = strings.TrimSpace(expression)
	if name == "" {
		return "", "", ErrJobNameRequired
	}
	if expression == "" {
		return "", "", ErrJobIntervalRequired
	}
	if task == nil {
		return "", "", ErrJobTaskRequired
	}
	return name, expression, nil
}

func (s *Scheduler) wrapTask(name string, task TaskFunc) TaskFunc {
	return func(ctx context.Context) error {
		if ctx == nil {
			ctx = context.Background()
		}
		startedAt := time.Now()
		if s == nil || s.leaseStore == nil || strings.TrimSpace(s.lockPrefix) == "" {
			err := task(ctx)
			if s != nil {
				s.recordExecution(time.Since(startedAt), true, schedulerOutcome(err))
			}
			return err
		}
		key := s.lockPrefix + strings.TrimSpace(name)
		lease, err := s.leaseStore.Acquire(ctx, key, s.leaseOwner, s.lockTTL)
		if errors.Is(err, redislock.ErrNotAcquired) {
			if s.logger != nil {
				s.logger.InfoContext(ctx, "skip scheduler job because distributed lease is held", "name", name, "lease_key", key)
			}
			s.recordExecution(time.Since(startedAt), false, "skipped")
			return nil
		}
		if err != nil {
			s.recordExecution(time.Since(startedAt), false, "error")
			return fmt.Errorf("scheduler acquire lease %s: %w", name, err)
		}

		taskCtx, cancelTask := context.WithCancel(ctx)
		stopRenewal := make(chan struct{})
		renewalDone := make(chan struct{})
		renewalErr := make(chan error, 1)
		go func() {
			defer close(renewalDone)
			interval := s.lockTTL / 3
			if interval < time.Millisecond {
				interval = time.Millisecond
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stopRenewal:
					return
				case <-taskCtx.Done():
					return
				case <-ticker.C:
					renewed, renewErr := s.leaseStore.Renew(taskCtx, lease, s.lockTTL)
					if renewErr != nil {
						if errors.Is(renewErr, context.Canceled) {
							select {
							case <-stopRenewal:
								return
							default:
							}
						}
						renewalErr <- fmt.Errorf("scheduler renew lease %s: %w", name, renewErr)
						cancelTask()
						return
					}
					lease = renewed
				}
			}
		}()

		taskErr := task(taskCtx)
		close(stopRenewal)
		cancelTask()
		<-renewalDone
		var lostErr error
		select {
		case lostErr = <-renewalErr:
		default:
		}

		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		releaseErr := s.leaseStore.Release(releaseCtx, lease)
		releaseCancel()
		if releaseErr != nil && !errors.Is(releaseErr, redislock.ErrLeaseLost) && s.logger != nil {
			s.logger.ErrorContext(ctx, "release scheduler lease failed", "name", name, "lease_key", key, "error", releaseErr)
		}
		if lostErr == nil && errors.Is(releaseErr, redislock.ErrLeaseLost) {
			lostErr = fmt.Errorf("scheduler release lease %s: %w", name, releaseErr)
		}
		resultErr := errors.Join(lostErr, taskErr)
		s.recordExecution(time.Since(startedAt), lostErr == nil, schedulerOutcome(resultErr))
		return resultErr
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

func newLeaseOwner() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("scheduler lease owner: %w", err)
	}
	return hex.EncodeToString(buf), nil
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
