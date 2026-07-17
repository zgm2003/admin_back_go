package crontask

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"admin_back_go/internal/infra/scheduler"
)

const DefaultReconcileInterval = 2 * time.Second

var (
	ErrReconcilerNotConfigured = errors.New("cron task reconciler is not configured")
	ErrReconcilerStarted       = errors.New("cron task reconciler already started")
	ErrUnregisteredSchedule    = errors.New("cron task schedule is not registered")
	ErrInvalidSchedule         = errors.New("cron task schedule is invalid")
)

type ScheduleController interface {
	AddCron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error
	UpdateCron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error
	Remove(name string) error
	List() []scheduler.Job
}

type ReconcileHealth struct {
	Healthy     bool
	LastAttempt time.Time
	LastSuccess time.Time
	Err         string
}

type ReconcilerOption func(*Reconciler)

func WithReconcileInterval(interval time.Duration) ReconcilerOption {
	return func(reconciler *Reconciler) {
		if interval > 0 {
			reconciler.interval = interval
		}
	}
}

type scheduleFingerprint struct {
	name       string
	expression string
	status     int
	updatedAt  time.Time
}

func (f scheduleFingerprint) equal(other scheduleFingerprint) bool {
	return f.name == other.name &&
		f.expression == other.expression &&
		f.status == other.status &&
		f.updatedAt.Equal(other.updatedAt)
}

type desiredSchedule struct {
	row         Task
	entry       RegistryEntry
	fingerprint scheduleFingerprint
	withSeconds bool
}

type Reconciler struct {
	service    *SchedulerService
	controller ScheduleController
	interval   time.Duration
	now        func() time.Time

	reconcileMu sync.Mutex
	stateMu     sync.RWMutex
	applied     map[string]scheduleFingerprint
	health      ReconcileHealth
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool
}

func NewReconciler(service *SchedulerService, controller ScheduleController, options ...ReconcilerOption) *Reconciler {
	reconciler := &Reconciler{
		service:    service,
		controller: controller,
		interval:   DefaultReconcileInterval,
		now:        time.Now,
		applied:    make(map[string]scheduleFingerprint),
		health:     ReconcileHealth{Err: "cron schedules have not been reconciled"},
	}
	for _, option := range options {
		if option != nil {
			option(reconciler)
		}
	}
	return reconciler
}

func (r *Reconciler) Start(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.stateMu.Lock()
	if r.started {
		r.stateMu.Unlock()
		return ErrReconcilerStarted
	}
	loopCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.started = true
	done := r.done
	r.stateMu.Unlock()

	_ = r.Reconcile(loopCtx)
	go r.run(loopCtx, done)
	return nil
}

func (r *Reconciler) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.Reconcile(ctx)
		}
	}
}

func (r *Reconciler) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.stateMu.RLock()
	cancel := r.cancel
	done := r.done
	started := r.started
	r.stateMu.RUnlock()
	if !started || cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.reconcileMu.Lock()
	defer r.reconcileMu.Unlock()
	attemptedAt := r.now()

	rows, err := r.service.repository.ListEnabled(ctx)
	if err != nil {
		err = fmt.Errorf("list enabled cron tasks: %w", err)
		r.recordResult(attemptedAt, err)
		return err
	}

	desired := make(map[string]desiredSchedule, len(rows))
	var reconcileErr error
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		expression := strings.TrimSpace(row.Cron)
		if _, duplicate := desired[name]; duplicate {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("%w: duplicate name %q", ErrInvalidSchedule, name))
			continue
		}
		entry, ok := r.service.registry.Lookup(name)
		if !ok {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("%w: %q", ErrUnregisteredSchedule, name))
			continue
		}
		if !isValidCronExpression(expression) {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("%w: %q (%s)", ErrInvalidSchedule, name, expression))
			continue
		}
		row.Name = name
		row.Cron = expression
		desired[name] = desiredSchedule{
			row:         row,
			entry:       entry,
			withSeconds: len(strings.Fields(expression)) == 6,
			fingerprint: scheduleFingerprint{name: name, expression: expression, status: row.Status, updatedAt: row.UpdatedAt},
		}
	}

	live := make(map[string]scheduler.Job)
	for _, job := range r.controller.List() {
		live[job.Name] = job
	}

	appliedNames := make([]string, 0, len(r.applied))
	for name := range r.applied {
		appliedNames = append(appliedNames, name)
	}
	sort.Strings(appliedNames)
	for _, name := range appliedNames {
		if _, keep := desired[name]; keep {
			continue
		}
		if _, exists := live[name]; exists {
			if removeErr := r.controller.Remove(name); removeErr != nil && !errors.Is(removeErr, scheduler.ErrJobNotFound) {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("remove cron schedule %q: %w", name, removeErr))
				continue
			}
		}
		delete(r.applied, name)
		delete(live, name)
	}

	desiredNames := make([]string, 0, len(desired))
	for name := range desired {
		desiredNames = append(desiredNames, name)
	}
	sort.Strings(desiredNames)
	for _, name := range desiredNames {
		schedule := desired[name]
		currentFingerprint, wasApplied := r.applied[name]
		_, isLive := live[name]
		if wasApplied && currentFingerprint.equal(schedule.fingerprint) && isLive {
			continue
		}
		if isLive && !wasApplied {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("add cron schedule %q: %w", name, scheduler.ErrJobAlreadyExists))
			continue
		}
		task := r.service.taskFunc(schedule.row, schedule.entry)
		if !isLive {
			if addErr := r.controller.AddCron(name, schedule.row.Cron, schedule.withSeconds, task); addErr != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("add cron schedule %q: %w", name, addErr))
				continue
			}
		} else {
			if updateErr := r.controller.UpdateCron(name, schedule.row.Cron, schedule.withSeconds, task); updateErr != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("update cron schedule %q: %w", name, updateErr))
				continue
			}
		}
		r.applied[name] = schedule.fingerprint
		live[name] = scheduler.Job{Name: name, Expression: schedule.row.Cron, WithSeconds: schedule.withSeconds}
	}

	r.recordResult(attemptedAt, reconcileErr)
	return reconcileErr
}

func (r *Reconciler) Health() ReconcileHealth {
	if r == nil {
		return ReconcileHealth{Err: ErrReconcilerNotConfigured.Error()}
	}
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.health
}

func (r *Reconciler) recordResult(attemptedAt time.Time, err error) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.health.LastAttempt = attemptedAt
	if err != nil {
		r.health.Healthy = false
		r.health.Err = err.Error()
		return
	}
	r.health.Healthy = true
	r.health.LastSuccess = attemptedAt
	r.health.Err = ""
}

func (r *Reconciler) validate() error {
	if r == nil || r.service == nil || r.service.repository == nil || r.controller == nil || r.interval <= 0 {
		return ErrReconcilerNotConfigured
	}
	return nil
}
