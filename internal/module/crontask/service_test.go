package crontask

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/module/notificationtask"
)

func TestServiceListDecoratesRegistryStatus(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		tasks: []Task{
			{ID: 1, Name: "notification_task_scheduler", Title: "通知任务调度器", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
			{ID: 2, Name: "payment_close_expired_order", Title: "支付超时关单", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, Handler: "app\\process\\Pay\\PaymentCloseExpiredOrderTask", CreatedAt: now, UpdatedAt: now},
			{ID: 3, Name: "disabled_task", Title: "禁用任务", Cron: "0 * * * * *", Status: CommonNo, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
			{ID: 4, Name: "bad_cron", Title: "错误表达式", Cron: "bad", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
		},
	}
	service := NewService(repo, NewDefaultRegistry())

	res, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("List returned appErr: %v", appErr)
	}
	if len(res.List) != 4 {
		t.Fatalf("expected 4 items, got %#v", res.List)
	}
	assertStatus(t, res.List[0], RegistryStatusRegistered)
	assertStatus(t, res.List[1], RegistryStatusMissing)
	assertStatus(t, res.List[2], RegistryStatusDisabled)
	assertStatus(t, res.List[3], RegistryStatusInvalidCron)
}

func TestServiceListUsesGoTaskTypeForRegisteredTaskHandler(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		tasks: []Task{{
			ID:        1,
			Name:      "notification_task_scheduler",
			Title:     "通知任务调度器",
			Cron:      "0 * * * * *",
			Handler:   "app\\process\\NotificationTaskScheduler",
			Status:    CommonYes,
			IsDel:     CommonNo,
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	service := NewService(repo, NewDefaultRegistry())

	res, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("List returned appErr: %v", appErr)
	}
	if len(res.List) != 1 {
		t.Fatalf("expected one item, got %#v", res.List)
	}
	item := res.List[0]
	if item.RegistryTaskType != notificationtask.TypeDispatchDueV1 {
		t.Fatalf("expected registry_task_type=%s, got %#v", notificationtask.TypeDispatchDueV1, item)
	}
	if item.Handler != notificationtask.TypeDispatchDueV1 {
		t.Fatalf("registered Go cron task must expose Go task type as handler, got %#v", item)
	}
}

func TestServiceListFiltersRegistryStatusBeforePagingAndTotal(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		tasks: []Task{
			{ID: 1, Name: "notification_task_scheduler", Title: "通知任务调度器", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
			{ID: 2, Name: "payment_close_expired_order", Title: "支付超时关单", Cron: "0 * * * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
			{ID: 3, Name: "legacy_missing_payment_task", Title: "遗留缺失支付任务", Cron: "0 0 2 * * *", Status: CommonYes, IsDel: CommonNo, CreatedAt: now, UpdatedAt: now},
		},
	}
	service := NewService(repo, NewDefaultRegistry())

	res, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 1, RegistryStatus: RegistryStatusMissing})
	if appErr != nil {
		t.Fatalf("List returned appErr: %v", appErr)
	}
	if len(res.List) != 1 || res.Page.Total != 2 || res.Page.TotalPage != 2 {
		t.Fatalf("expected registry_status filter before paging, got list=%#v page=%#v", res.List, res.Page)
	}
	if res.List[0].Name != "payment_close_expired_order" {
		t.Fatalf("unexpected first missing task after paging: %#v", res.List[0])
	}
}

func TestServiceListTreatsPaymentOrderCronAsMissingInConfigOnlySlice(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		tasks: []Task{{
			ID:        2,
			Name:      "payment_close_expired_order",
			Title:     "支付超时关单",
			Cron:      "0 * * * * *",
			Handler:   "payment:close-expired-order:v1",
			Status:    CommonYes,
			IsDel:     CommonNo,
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	service := NewService(repo, NewDefaultRegistry())

	res, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("List returned appErr: %v", appErr)
	}
	item := res.List[0]
	if item.RegistryStatus != RegistryStatusMissing {
		t.Fatalf("payment order cron must be missing in config-only slice, got %#v", item)
	}
	if item.RegistryTaskType != "" || item.Handler != "payment:close-expired-order:v1" {
		t.Fatalf("missing retired payment task must preserve stored handler only, got %#v", item)
	}
}

func TestServiceListTreatsOldPayCronNamesAsMissing(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		tasks: []Task{{
			ID:        5,
			Name:      "pay_fulfillment_retry",
			Title:     "支付履约重试",
			Cron:      "0 */2 * * * *",
			Handler:   "app\\process\\Pay\\PayFulfillmentRetryTask",
			Status:    CommonYes,
			IsDel:     CommonNo,
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	service := NewService(repo, NewDefaultRegistry())

	res, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("List returned appErr: %v", appErr)
	}
	item := res.List[0]
	if item.RegistryStatus != RegistryStatusMissing {
		t.Fatalf("old pay cron task must be missing after payment rebuild, got %#v", item)
	}
	if item.RegistryTaskType != "" || item.Handler != "app\\process\\Pay\\PayFulfillmentRetryTask" {
		t.Fatalf("missing old pay task must preserve legacy handler only, got %#v", item)
	}
}

func TestServiceCreateRejectsDuplicateName(t *testing.T) {
	service := NewService(&fakeRepository{nameExists: true}, NewDefaultRegistry())
	_, appErr := service.Create(context.Background(), SaveInput{Name: "notification_task_scheduler", Title: "通知", Cron: "0 * * * * *", Status: CommonYes})
	if appErr == nil {
		t.Fatalf("expected duplicate name app error")
	}
}

func TestServiceCreateRejectsInvalidCron(t *testing.T) {
	service := NewService(&fakeRepository{}, NewDefaultRegistry())
	_, appErr := service.Create(context.Background(), SaveInput{Name: "demo_task", Title: "demo", Cron: "bad", Status: CommonYes})
	if appErr == nil {
		t.Fatalf("expected invalid cron app error")
	}
}

func TestServiceUpdateRejectsNameChange(t *testing.T) {
	repo := &fakeRepository{
		tasks: []Task{{ID: 9, Name: "notification_task_scheduler", Title: "通知", Cron: "0 * * * * *", Status: CommonYes}},
	}
	service := NewService(repo, NewDefaultRegistry())

	appErr := service.Update(context.Background(), 9, SaveInput{Name: "payment_close_expired_order", Title: "通知", Cron: "0 * * * * *", Status: CommonYes})
	if appErr == nil {
		t.Fatalf("expected name change rejection")
	}
	if repo.updated {
		t.Fatalf("repository update must not be called when cron task name changes")
	}
}

func TestServiceLogsMapsStatusAndDates(t *testing.T) {
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)
	end := start.Add(time.Second)
	duration := int64(1000)
	repo := &fakeRepository{
		logs: []TaskLog{{ID: 9, TaskID: 1, TaskName: "notification_task_scheduler", StartTime: &start, EndTime: &end, DurationMS: &duration, Status: LogStatusSuccess, Result: "queued", CreatedAt: start}},
	}
	service := NewService(repo, NewDefaultRegistry())
	res, appErr := service.Logs(context.Background(), LogsQuery{TaskID: 1, CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("Logs returned appErr: %v", appErr)
	}
	if len(res.List) != 1 || res.List[0].StatusName != "成功" || res.List[0].StartTime == nil {
		t.Fatalf("unexpected logs response: %#v", res.List)
	}
}

func assertStatus(t *testing.T, item ListItem, want string) {
	t.Helper()
	if item.RegistryStatus != want {
		t.Fatalf("task %s expected registry_status=%s, got %#v", item.Name, want, item)
	}
}

type fakeRepository struct {
	tasks       []Task
	logs        []TaskLog
	nameExists  bool
	createID    int64
	err         error
	startedLogs []Task
	endedLogs   []fakeEndedLog
	updated     bool
}

type fakeEndedLog struct {
	logID   int64
	success bool
	result  string
	errMsg  string
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	return f.tasks, int64(len(f.tasks)), f.err
}
func (f *fakeRepository) ListAll(ctx context.Context, query ListQuery) ([]Task, error) {
	return f.tasks, f.err
}
func (f *fakeRepository) NameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	return f.nameExists, f.err
}
func (f *fakeRepository) Create(ctx context.Context, row Task) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.createID == 0 {
		return 1, nil
	}
	return f.createID, nil
}
func (f *fakeRepository) Get(ctx context.Context, id int64) (*Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, task := range f.tasks {
		if task.ID == id {
			return &task, nil
		}
	}
	return nil, ErrTaskNotFound
}
func (f *fakeRepository) Update(ctx context.Context, id int64, row Task) error {
	f.updated = true
	return f.err
}
func (f *fakeRepository) UpdateStatus(ctx context.Context, id int64, status int) error { return f.err }
func (f *fakeRepository) Delete(ctx context.Context, ids []int64) error                { return f.err }
func (f *fakeRepository) Logs(ctx context.Context, query LogsQuery) ([]TaskLog, int64, error) {
	return f.logs, int64(len(f.logs)), f.err
}
func (f *fakeRepository) ListEnabled(ctx context.Context) ([]Task, error) { return f.tasks, f.err }
func (f *fakeRepository) LogStart(ctx context.Context, task Task, now time.Time) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.startedLogs = append(f.startedLogs, task)
	return int64(len(f.startedLogs)), nil
}
func (f *fakeRepository) LogEnd(ctx context.Context, logID int64, success bool, result string, errMsg string, now time.Time) error {
	if errors.Is(f.err, ErrTaskNotFound) {
		return nil
	}
	f.endedLogs = append(f.endedLogs, fakeEndedLog{logID: logID, success: success, result: result, errMsg: errMsg})
	return f.err
}
