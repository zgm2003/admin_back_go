package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"testing"
	"time"

	"admin_back_go/internal/infra/scheduler"
	"admin_back_go/internal/infra/taskqueue"
	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/export"
	notificationtask "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/module/payment"
)

func TestNewRegistryOwnsEveryCurrentVersionedTask(t *testing.T) {
	registry, err := NewRegistry(Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		aichat.ConversationReplyTaskName,
		aiimage.TypeGenerateV1,
		aichat.TypeRunTimeoutV1,
		auth.TypeAuthLoginLogV1,
		exporttask.TypeCleanupExpiredV1,
		exporttask.TypeRunV1,
		notificationtask.TypeDispatchDueV1,
		notificationtask.TypeSendTaskV1,
		payment.TypeCloseExpiredOrderV1,
		payment.TypeSyncPendingOrderV1,
		TypeSystemNoopV1,
	}
	slices.Sort(want)
	if got := registry.Types(); !slices.Equal(got, want) {
		t.Fatalf("registered types mismatch\n got: %v\nwant: %v", got, want)
	}

	policies := []struct {
		taskType  string
		queue     string
		maxRetry  int
		timeout   time.Duration
		uniqueTTL time.Duration
	}{
		{TypeSystemNoopV1, taskqueue.QueueDefault, 3, 30 * time.Second, 0},
		{auth.TypeAuthLoginLogV1, taskqueue.QueueCritical, 3, 30 * time.Second, 0},
		{aichat.ConversationReplyTaskName, taskqueue.QueueDefault, 2, 5 * time.Minute, 0},
		{aichat.TypeRunTimeoutV1, taskqueue.QueueDefault, 3, 30 * time.Second, 55 * time.Second},
		{aiimage.TypeGenerateV1, taskqueue.QueueLow, 2, 10 * time.Minute, 0},
		{exporttask.TypeRunV1, taskqueue.QueueLow, 3, 5 * time.Minute, 0},
		{exporttask.TypeCleanupExpiredV1, taskqueue.QueueLow, 3, time.Minute, 5 * time.Minute},
		{notificationtask.TypeDispatchDueV1, taskqueue.QueueDefault, 3, 30 * time.Second, 55 * time.Second},
		{notificationtask.TypeSendTaskV1, taskqueue.QueueDefault, 3, 30 * time.Second, 0},
		{payment.TypeSyncPendingOrderV1, taskqueue.QueueDefault, 3, 30 * time.Second, 55 * time.Second},
		{payment.TypeCloseExpiredOrderV1, taskqueue.QueueDefault, 3, 30 * time.Second, 55 * time.Second},
	}
	for _, wantPolicy := range policies {
		_, policy, err := registry.Task(wantPolicy.taskType, nil)
		if err != nil {
			t.Fatal(err)
		}
		if policy.Queue != wantPolicy.queue || policy.MaxRetry != wantPolicy.maxRetry || policy.Timeout != wantPolicy.timeout || policy.UniqueTTL != wantPolicy.uniqueTTL {
			t.Fatalf("%s policy mismatch: got %+v want %+v", wantPolicy.taskType, policy, wantPolicy)
		}
	}
}

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

func TestRegisterHandlesAIConversationReplyTask(t *testing.T) {
	service := &fakeAIChatJobService{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:        slog.Default(),
		AIChatService: service,
	})

	task, err := aichat.NewConversationReplyTask(aichat.ConversationReplyPayload{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 8, RequestID: "rid"})
	if err != nil {
		t.Fatalf("NewConversationReplyTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask conversation reply returned error: %v", err)
	}
	if service.reply.ConversationID != 3 || service.reply.UserMessageID != 8 || service.reply.RequestID != "rid" {
		t.Fatalf("expected conversation reply payload, got %#v", service.reply)
	}
}

func TestRegisterHandlesAIChatRunTimeoutTask(t *testing.T) {
	service := &fakeAIChatJobService{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:        slog.Default(),
		AIChatService: service,
	})

	task, err := aichat.NewRunTimeoutTask(aichat.RunTimeoutPayload{Limit: 9})
	if err != nil {
		t.Fatalf("NewRunTimeoutTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask run timeout returned error: %v", err)
	}
	if service.timeoutLimit != 9 {
		t.Fatalf("expected timeout limit 9, got %d", service.timeoutLimit)
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
type fakeAIChatJobService struct {
	reply        aichat.ConversationReplyInput
	timeoutLimit int
}

func (f *fakeAIChatJobService) ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	f.reply = input
	return &aichat.ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: 22}, nil
}

func (f *fakeAIChatJobService) TimeoutRuns(ctx context.Context, input aichat.RunTimeoutInput) (*aichat.RunTimeoutResult, error) {
	f.timeoutLimit = input.Limit
	return &aichat.RunTimeoutResult{}, nil
}

type fakeNotificationTaskJobService struct {
	dispatchLimit int
	sendTaskID    int64
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

func (f *fakeAuthRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	return nil
}

func (f *fakeAuthRepository) RecordLoginAttempt(ctx context.Context, attempt auth.LoginAttempt) error {
	f.attempts = append(f.attempts, attempt)
	return nil
}

func TestRegisterHandlesExportTaskHandlers(t *testing.T) {
	service := &fakeExportTaskJobService{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:            slog.Default(),
		ExportTaskService: service,
	})

	task, err := exporttask.NewRunTask(exporttask.RunPayload{TaskID: 7, Kind: exporttask.KindUserList, UserID: 9, Platform: "admin", IDs: []int64{3}})
	if err != nil {
		t.Fatalf("NewRunTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if service.input.TaskID != 7 || service.input.Kind != exporttask.KindUserList || service.input.UserID != 9 {
		t.Fatalf("unexpected export task input: %#v", service.input)
	}
}

type fakeExportTaskJobService struct {
	input exporttask.RunInput
}

func (f *fakeExportTaskJobService) Run(ctx context.Context, input exporttask.RunInput) error {
	f.input = input
	return nil
}

func (f *fakeExportTaskJobService) CleanupExpired(ctx context.Context) error { return nil }
