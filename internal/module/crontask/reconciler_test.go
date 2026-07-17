package crontask

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/scheduler"
)

func TestReconcileConvergesCreateUpdateDisableAndDeleteWithinFiveSeconds(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	repository := &mutableScheduleRepository{}
	repository.set([]Task{{
		ID: 1, Name: "notification_task_scheduler", Cron: "*/5 * * * * *",
		Status: CommonYes, IsDel: CommonNo, UpdatedAt: now,
	}}, nil)
	controller := newFakeScheduleController()
	service := NewSchedulerService(repository, NewDefaultRegistry(), &fakeEnqueuer{}, slog.Default())
	reconciler := NewReconciler(service, controller, WithReconcileInterval(20*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := reconciler.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = reconciler.Shutdown(context.Background()) })
	started := time.Now()

	eventuallySchedule(t, time.Second, func(jobs map[string]scheduler.Job) bool {
		return jobs["notification_task_scheduler"].Expression == "*/5 * * * * *"
	}, controller)

	repository.set([]Task{{
		ID: 1, Name: "notification_task_scheduler", Cron: "*/10 * * * * *",
		Status: CommonYes, IsDel: CommonNo, UpdatedAt: now.Add(time.Second),
	}}, nil)
	eventuallySchedule(t, time.Second, func(jobs map[string]scheduler.Job) bool {
		return jobs["notification_task_scheduler"].Expression == "*/10 * * * * *"
	}, controller)

	// Disabled and deleted rows both disappear from ListEnabled and must remove the live job.
	repository.set(nil, nil)
	eventuallySchedule(t, time.Second, func(jobs map[string]scheduler.Job) bool {
		_, exists := jobs["notification_task_scheduler"]
		return !exists
	}, controller)

	repository.set([]Task{{
		ID: 2, Name: "payment_close_expired_order", Cron: "*/15 * * * * *",
		Status: CommonYes, IsDel: CommonNo, UpdatedAt: now.Add(2 * time.Second),
	}}, nil)
	eventuallySchedule(t, time.Second, func(jobs map[string]scheduler.Job) bool {
		return jobs["payment_close_expired_order"].Expression == "*/15 * * * * *"
	}, controller)

	if elapsed := time.Since(started); elapsed >= 5*time.Second {
		t.Fatalf("schedule convergence exceeded five seconds: %s", elapsed)
	}
	if health := reconciler.Health(); !health.Healthy || health.LastSuccess.IsZero() || health.Err != "" {
		t.Fatalf("expected healthy reconciler, got %#v", health)
	}
}

func TestReconcileUnknownTaskInvalidCronAndRepositoryFailureAreUnhealthy(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	repository := &mutableScheduleRepository{}
	controller := newFakeScheduleController()
	service := NewSchedulerService(repository, NewDefaultRegistry(), &fakeEnqueuer{}, slog.Default())
	reconciler := NewReconciler(service, controller)

	repository.set([]Task{{ID: 1, Name: "not_registered", Cron: "*/5 * * * * *", Status: CommonYes, UpdatedAt: now}}, nil)
	if err := reconciler.Reconcile(context.Background()); !errors.Is(err, ErrUnregisteredSchedule) {
		t.Fatalf("expected ErrUnregisteredSchedule, got %v", err)
	}
	if health := reconciler.Health(); health.Healthy || !strings.Contains(health.Err, "not_registered") {
		t.Fatalf("unknown task must be unhealthy with task identity, got %#v", health)
	}

	repository.set([]Task{{ID: 2, Name: "notification_task_scheduler", Cron: "not a cron", Status: CommonYes, UpdatedAt: now}}, nil)
	if err := reconciler.Reconcile(context.Background()); !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("expected ErrInvalidSchedule, got %v", err)
	}
	if health := reconciler.Health(); health.Healthy || !strings.Contains(health.Err, "notification_task_scheduler") {
		t.Fatalf("invalid cron must be unhealthy, got %#v", health)
	}

	repository.set([]Task{{ID: 3, Name: "notification_task_scheduler", Cron: "*/5 * * * * *", Status: CommonYes, UpdatedAt: now}}, nil)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("valid reconcile returned error: %v", err)
	}
	before := controller.snapshot()
	repository.set(nil, errors.New("mysql unavailable"))
	if err := reconciler.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "mysql unavailable") {
		t.Fatalf("expected repository error, got %v", err)
	}
	if health := reconciler.Health(); health.Healthy || !strings.Contains(health.Err, "mysql unavailable") {
		t.Fatalf("repository error must be unhealthy, got %#v", health)
	}
	after := controller.snapshot()
	if len(before) != len(after) || after["notification_task_scheduler"].Expression != before["notification_task_scheduler"].Expression {
		t.Fatalf("repository failure must not mutate live schedules: before=%#v after=%#v", before, after)
	}
}

func TestReconcileUpdatedAtChangeRefreshesCallbackEvenWhenCronIsUnchanged(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	repository := &mutableScheduleRepository{}
	controller := newFakeScheduleController()
	service := NewSchedulerService(repository, NewDefaultRegistry(), &fakeEnqueuer{}, slog.Default())
	reconciler := NewReconciler(service, controller)

	repository.set([]Task{{ID: 1, Name: "notification_task_scheduler", Cron: "*/5 * * * * *", Status: CommonYes, UpdatedAt: now}}, nil)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	repository.set([]Task{{ID: 1, Name: "notification_task_scheduler", Cron: "*/5 * * * * *", Status: CommonYes, UpdatedAt: now.Add(time.Second)}}, nil)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	controller.mu.Lock()
	updates := controller.updateCalls
	controller.mu.Unlock()
	if updates != 1 {
		t.Fatalf("updated_at change must refresh callback exactly once, got %d updates", updates)
	}
}

func TestReconcileDoesNotReplaceUnownedStaticSchedule(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	repository := &mutableScheduleRepository{}
	repository.set([]Task{{
		ID: 1, Name: "notification_task_scheduler", Cron: "*/5 * * * * *",
		Status: CommonYes, UpdatedAt: now,
	}}, nil)
	controller := newFakeScheduleController()
	controller.jobs["notification_task_scheduler"] = scheduler.Job{
		Name: "notification_task_scheduler", Expression: "static-interval",
	}
	service := NewSchedulerService(repository, NewDefaultRegistry(), &fakeEnqueuer{}, slog.Default())
	reconciler := NewReconciler(service, controller)

	err := reconciler.Reconcile(context.Background())
	if !errors.Is(err, scheduler.ErrJobAlreadyExists) {
		t.Fatalf("expected static schedule collision, got %v", err)
	}
	jobs := controller.snapshot()
	if jobs["notification_task_scheduler"].Expression != "static-interval" {
		t.Fatalf("reconciler replaced an unowned static schedule: %#v", jobs)
	}
	controller.mu.Lock()
	updates := controller.updateCalls
	controller.mu.Unlock()
	if updates != 0 {
		t.Fatalf("unowned static schedule was updated %d times", updates)
	}
}

func TestReconcileMySQLScheduleCreateUpdateDisableDelete(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for MySQL schedule reconciliation test")
	}
	client, err := database.Open(config.MySQLConfig{DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open test MySQL: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	tx := client.Gorm.Begin()
	if tx.Error != nil {
		t.Fatalf("begin test transaction: %v", tx.Error)
	}
	if err := tx.Exec("CREATE TEMPORARY TABLE p05_cron_task LIKE admin.cron_task").Error; err != nil {
		_ = tx.Rollback().Error
		t.Fatalf("create isolated cron_task fixture: %v", err)
	}
	if err := tx.Exec("ALTER TABLE p05_cron_task RENAME TO cron_task").Error; err != nil {
		_ = tx.Exec("DROP TEMPORARY TABLE IF EXISTS p05_cron_task").Error
		_ = tx.Rollback().Error
		t.Fatalf("activate isolated cron_task fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Exec("DROP TEMPORARY TABLE IF EXISTS cron_task").Error
		_ = tx.Rollback().Error
	})

	const name = "notification_task_scheduler"
	repository := NewGormRepository(&database.Client{Gorm: tx})
	controller := newFakeScheduleController()
	service := NewSchedulerService(repository, NewDefaultRegistry(), &fakeEnqueuer{}, slog.Default())
	reconciler := NewReconciler(service, controller)
	started := time.Now()

	id, err := repository.Create(context.Background(), Task{
		Name: name, Title: "P05 reconcile test", Cron: "*/5 * * * * *",
		Handler: "notification:dispatch-due:v1", Status: CommonYes, IsDel: CommonNo,
	})
	if err != nil {
		t.Fatalf("create DB schedule: %v", err)
	}
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile create: %v", err)
	}
	if got := controller.snapshot()[name].Expression; got != "*/5 * * * * *" {
		t.Fatalf("created schedule did not converge: %q", got)
	}

	if err := repository.Update(context.Background(), id, Task{
		Title: "P05 reconcile test", Cron: "*/10 * * * * *",
		Handler: "notification:dispatch-due:v1", Status: CommonYes,
	}); err != nil {
		t.Fatalf("update DB schedule: %v", err)
	}
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile update: %v", err)
	}
	if got := controller.snapshot()[name].Expression; got != "*/10 * * * * *" {
		t.Fatalf("updated schedule did not converge: %q", got)
	}

	if err := repository.UpdateStatus(context.Background(), id, CommonNo); err != nil {
		t.Fatalf("disable DB schedule: %v", err)
	}
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile disable: %v", err)
	}
	if _, exists := controller.snapshot()[name]; exists {
		t.Fatal("disabled DB schedule remained live")
	}

	if err := repository.UpdateStatus(context.Background(), id, CommonYes); err != nil {
		t.Fatalf("re-enable DB schedule: %v", err)
	}
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile re-enable: %v", err)
	}
	if _, exists := controller.snapshot()[name]; !exists {
		t.Fatal("re-enabled DB schedule did not return")
	}
	if err := repository.Delete(context.Background(), []int64{id}); err != nil {
		t.Fatalf("delete DB schedule: %v", err)
	}
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile delete: %v", err)
	}
	if _, exists := controller.snapshot()[name]; exists {
		t.Fatal("deleted DB schedule remained live")
	}
	if elapsed := time.Since(started); elapsed >= 5*time.Second {
		t.Fatalf("MySQL schedule lifecycle exceeded five seconds: %s", elapsed)
	}
}

type mutableScheduleRepository struct {
	Repository
	mu    sync.RWMutex
	tasks []Task
	err   error
}

func (r *mutableScheduleRepository) set(tasks []Task, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append([]Task(nil), tasks...)
	r.err = err
}

func (r *mutableScheduleRepository) ListEnabled(context.Context) ([]Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Task(nil), r.tasks...), r.err
}

type fakeScheduleController struct {
	mu          sync.Mutex
	jobs        map[string]scheduler.Job
	updateCalls int
}

func newFakeScheduleController() *fakeScheduleController {
	return &fakeScheduleController{jobs: make(map[string]scheduler.Job)}
}

func (f *fakeScheduleController) AddCron(name string, expression string, withSeconds bool, _ scheduler.TaskFunc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.jobs[name]; exists {
		return scheduler.ErrJobAlreadyExists
	}
	f.jobs[name] = scheduler.Job{Name: name, Expression: expression, WithSeconds: withSeconds}
	return nil
}

func (f *fakeScheduleController) UpdateCron(name string, expression string, withSeconds bool, _ scheduler.TaskFunc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.jobs[name]; !exists {
		return scheduler.ErrJobNotFound
	}
	f.updateCalls++
	f.jobs[name] = scheduler.Job{Name: name, Expression: expression, WithSeconds: withSeconds}
	return nil
}

func (f *fakeScheduleController) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.jobs[name]; !exists {
		return scheduler.ErrJobNotFound
	}
	delete(f.jobs, name)
	return nil
}

func (f *fakeScheduleController) List() []scheduler.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	jobs := make([]scheduler.Job, 0, len(f.jobs))
	for _, job := range f.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (f *fakeScheduleController) snapshot() map[string]scheduler.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[string]scheduler.Job, len(f.jobs))
	for name, job := range f.jobs {
		result[name] = job
	}
	return result
}

func eventuallySchedule(t *testing.T, timeout time.Duration, condition func(map[string]scheduler.Job) bool, controller *fakeScheduleController) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition(controller.snapshot()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("schedule did not converge, jobs=%#v", controller.snapshot())
}

var _ ScheduleController = (*fakeScheduleController)(nil)
