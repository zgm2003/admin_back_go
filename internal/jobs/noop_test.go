package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/notificationtask"
	"admin_back_go/internal/module/payruntime"
	"admin_back_go/internal/platform/scheduler"
	"admin_back_go/internal/platform/taskqueue"
)

func TestNewNoopTaskUsesVersionedType(t *testing.T) {
	task, err := NewNoopTask(NoopPayload{Message: "hello"})
	if err != nil {
		t.Fatalf("NewNoopTask returned error: %v", err)
	}

	if task.Type != TypeSystemNoopV1 {
		t.Fatalf("expected type %s, got %s", TypeSystemNoopV1, task.Type)
	}
	var payload NoopPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message != "hello" {
		t.Fatalf("expected message hello, got %q", payload.Message)
	}
}

func TestRegisterHandlesNoopTask(t *testing.T) {
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{Logger: slog.Default()})

	task, err := NewNoopTask(NoopPayload{Message: "ok"})
	if err != nil {
		t.Fatalf("NewNoopTask returned error: %v", err)
	}

	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
}

func TestRegisterHandlesAuthLoginLogTask(t *testing.T) {
	repo := &fakeAuthRepository{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:         slog.Default(),
		AuthRepository: repo,
	})

	task, err := auth.NewLoginLogTask(auth.LoginAttempt{
		LoginAccount: "15671628271",
		LoginType:    auth.LoginTypePhone,
		Platform:     "admin",
		IsSuccess:    2,
		Reason:       "invalid_code",
	})
	if err != nil {
		t.Fatalf("NewLoginLogTask returned error: %v", err)
	}

	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].Reason != "invalid_code" {
		t.Fatalf("expected auth login log handler to write repository, got %#v", repo.attempts)
	}
}

func TestRegisterHandlesNotificationTaskHandlers(t *testing.T) {
	service := &fakeNotificationTaskJobService{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:                  slog.Default(),
		NotificationTaskService: service,
	})

	task, err := notificationtask.NewDispatchDueTask(notificationtask.DispatchDuePayload{Limit: 9})
	if err != nil {
		t.Fatalf("NewDispatchDueTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask dispatch returned error: %v", err)
	}
	if service.dispatchLimit != 9 {
		t.Fatalf("expected dispatch limit 9, got %d", service.dispatchLimit)
	}

	task, err = notificationtask.NewSendTask(77)
	if err != nil {
		t.Fatalf("NewSendTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask send returned error: %v", err)
	}
	if service.sendTaskID != 77 {
		t.Fatalf("expected send task id 77, got %d", service.sendTaskID)
	}
}

func TestRegisterHandlesPayRuntimeCronHandlers(t *testing.T) {
	service := &fakePayRuntimeJobService{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:            slog.Default(),
		PayRuntimeService: service,
	})

	closeTask, err := payruntime.NewCloseExpiredOrderTask(payruntime.CloseExpiredOrderPayload{Limit: 11})
	if err != nil {
		t.Fatalf("NewCloseExpiredOrderTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), closeTask); err != nil {
		t.Fatalf("ProcessProjectTask close expired returned error: %v", err)
	}
	if service.closeLimit != 11 {
		t.Fatalf("expected close limit 11, got %d", service.closeLimit)
	}

	syncTask, err := payruntime.NewSyncPendingTransactionTask(payruntime.SyncPendingTransactionPayload{Limit: 12})
	if err != nil {
		t.Fatalf("NewSyncPendingTransactionTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), syncTask); err != nil {
		t.Fatalf("ProcessProjectTask sync pending returned error: %v", err)
	}
	if service.syncLimit != 12 {
		t.Fatalf("expected sync limit 12, got %d", service.syncLimit)
	}
}

func TestRegisterScheduleDefinitionsOnlyEnqueuesTaskWhenTriggered(t *testing.T) {
	registrar := &fakeScheduleRegistrar{}
	enqueuer := &fakeEnqueuer{}
	buildCount := 0

	err := registerScheduleDefinitions(registrar, enqueuer, slog.Default(), []ScheduledTaskDefinition{
		{
			Name:  "system-noop-probe",
			Every: time.Minute,
			BuildTask: func() (taskqueue.Task, error) {
				buildCount++
				return NewNoopTask(NoopPayload{Message: "tick"})
			},
		},
	})
	if err != nil {
		t.Fatalf("registerScheduleDefinitions returned error: %v", err)
	}
	if len(registrar.everyCalls) != 1 {
		t.Fatalf("expected one interval schedule, got %#v", registrar.everyCalls)
	}
	if buildCount != 0 {
		t.Fatalf("expected registration not to build or run task, got buildCount=%d", buildCount)
	}

	if err := registrar.everyCalls[0].task(context.Background()); err != nil {
		t.Fatalf("scheduled task returned error: %v", err)
	}
	if buildCount != 1 {
		t.Fatalf("expected task builder to run once on trigger, got %d", buildCount)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("expected one enqueued task, got %#v", enqueuer.tasks)
	}
	if enqueuer.tasks[0].Type != TypeSystemNoopV1 {
		t.Fatalf("expected task type %s, got %s", TypeSystemNoopV1, enqueuer.tasks[0].Type)
	}
}

func TestRegisterSchedulesDoesNotRegisterStaticNotificationDispatchDue(t *testing.T) {
	registrar := &fakeScheduleRegistrar{}
	enqueuer := &fakeEnqueuer{}

	err := RegisterSchedules(registrar, enqueuer, slog.Default())
	if err != nil {
		t.Fatalf("RegisterSchedules returned error: %v", err)
	}
	if len(registrar.everyCalls) != 0 || len(registrar.cronCalls) != 0 {
		t.Fatalf("static schedules must stay empty; DB-backed cron task service owns registration, every=%#v cron=%#v", registrar.everyCalls, registrar.cronCalls)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("registration should not enqueue immediately, got %#v", enqueuer.tasks)
	}
}

type fakeAuthRepository struct {
	attempts []auth.LoginAttempt
}

type fakeNotificationTaskJobService struct {
	dispatchLimit int
	sendTaskID    int64
}

type fakePayRuntimeJobService struct {
	closeLimit int
	syncLimit  int
}

func (f *fakePayRuntimeJobService) CloseExpiredOrders(ctx context.Context, input payruntime.CloseExpiredOrderInput) (*payruntime.CloseExpiredOrderResult, error) {
	f.closeLimit = input.Limit
	return &payruntime.CloseExpiredOrderResult{}, nil
}

func (f *fakePayRuntimeJobService) SyncPendingTransactions(ctx context.Context, input payruntime.SyncPendingTransactionInput) (*payruntime.SyncPendingTransactionResult, error) {
	f.syncLimit = input.Limit
	return &payruntime.SyncPendingTransactionResult{}, nil
}

func (f *fakeNotificationTaskJobService) DispatchDue(ctx context.Context, input notificationtask.DispatchDueInput) (*notificationtask.DispatchDueResult, error) {
	f.dispatchLimit = input.Limit
	return &notificationtask.DispatchDueResult{Claimed: 1, Queued: 1}, nil
}

func (f *fakeNotificationTaskJobService) SendTask(ctx context.Context, input notificationtask.SendTaskInput) (*notificationtask.SendTaskResult, error) {
	f.sendTaskID = input.TaskID
	return &notificationtask.SendTaskResult{TaskID: input.TaskID, Sent: 1}, nil
}

type fakeScheduleRegistrar struct {
	everyCalls []registeredEveryCall
	cronCalls  []registeredCronCall
}

type registeredEveryCall struct {
	name     string
	interval time.Duration
	task     scheduler.TaskFunc
}

type registeredCronCall struct {
	name        string
	expression  string
	withSeconds bool
	task        scheduler.TaskFunc
}

func (f *fakeScheduleRegistrar) Every(name string, interval time.Duration, task scheduler.TaskFunc) error {
	f.everyCalls = append(f.everyCalls, registeredEveryCall{
		name:     name,
		interval: interval,
		task:     task,
	})
	return nil
}

func (f *fakeScheduleRegistrar) Cron(name string, expression string, withSeconds bool, task scheduler.TaskFunc) error {
	f.cronCalls = append(f.cronCalls, registeredCronCall{
		name:        name,
		expression:  expression,
		withSeconds: withSeconds,
		task:        task,
	})
	return nil
}

type fakeEnqueuer struct {
	tasks []taskqueue.Task
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	return taskqueue.EnqueueResult{
		ID:    "test-task-id",
		Queue: task.Queue,
		Type:  task.Type,
	}, nil
}

func (f *fakeAuthRepository) WithTx(ctx context.Context, fn func(auth.Repository) error) error {
	return fn(f)
}

func (f *fakeAuthRepository) FindCredentialByEmail(ctx context.Context, email string) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindCredentialByPhone(ctx context.Context, phone string) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindCredentialByID(ctx context.Context, id int64) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindDefaultRole(ctx context.Context) (*auth.DefaultRole, error) {
	return nil, nil
}

func (f *fakeAuthRepository) CreateUser(ctx context.Context, input auth.CreateUserInput) (int64, error) {
	return 0, nil
}

func (f *fakeAuthRepository) CreateProfile(ctx context.Context, input auth.CreateProfileInput) error {
	return nil
}

func (f *fakeAuthRepository) RecordLoginAttempt(ctx context.Context, attempt auth.LoginAttempt) error {
	f.attempts = append(f.attempts, attempt)
	return nil
}
