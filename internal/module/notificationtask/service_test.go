package notificationtask

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/taskqueue"
)

type fakeRepository struct {
	rows           []Task
	total          int64
	counts         map[int]int64
	gotList        ListQuery
	gotCount       StatusCountQuery
	created        Task
	createdID      int64
	targetCount    int
	gotTargetType  int
	gotTargetIDs   []int64
	getRow         *Task
	cancelAffected int64
	deleteAffected int64
	dueIDs         []int64
	gotDueLimit    int
	sendTask       *Task
	sendClaimed    bool
	targetUserIDs  []int64
	inserted       []Notification
	progressSent   []int
	progressTotal  []int
	successSent    int
	successTotal   int
	failedMsg      string
	err            error
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Task, int64, error) {
	f.gotList = query
	return f.rows, f.total, f.err
}

func (f *fakeRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	f.gotCount = query
	return f.counts, f.err
}

func (f *fakeRepository) Create(ctx context.Context, row Task) (int64, error) {
	f.created = row
	if f.createdID == 0 {
		f.createdID = 10
	}
	return f.createdID, f.err
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Task, error) {
	return f.getRow, f.err
}

func (f *fakeRepository) CancelPending(ctx context.Context, id int64) (int64, error) {
	return f.cancelAffected, f.err
}

func (f *fakeRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return f.deleteAffected, f.err
}

func (f *fakeRepository) CountTargetUsers(ctx context.Context, targetType int, targetIDs []int64) (int, error) {
	f.gotTargetType = targetType
	f.gotTargetIDs = append([]int64{}, targetIDs...)
	return f.targetCount, f.err
}

func (f *fakeRepository) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	f.gotDueLimit = limit
	return f.dueIDs, f.err
}

func (f *fakeRepository) ClaimSendTask(ctx context.Context, id int64) (*Task, bool, error) {
	return f.sendTask, f.sendClaimed, f.err
}

func (f *fakeRepository) TargetUserIDs(ctx context.Context, task Task) ([]int64, error) {
	return f.targetUserIDs, f.err
}

func (f *fakeRepository) InsertNotifications(ctx context.Context, rows []Notification) error {
	f.inserted = append(f.inserted, rows...)
	return f.err
}

func (f *fakeRepository) UpdateProgress(ctx context.Context, id int64, sentCount int, totalCount int) error {
	f.progressSent = append(f.progressSent, sentCount)
	f.progressTotal = append(f.progressTotal, totalCount)
	return f.err
}

func (f *fakeRepository) MarkSuccess(ctx context.Context, id int64, sentCount int, totalCount int) error {
	f.successSent = sentCount
	f.successTotal = totalCount
	return f.err
}

func (f *fakeRepository) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	f.failedMsg = errMsg
	return nil
}

type fakeEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

type fakeRealtimePublisher struct {
	publications []platformrealtime.Publication
	err          error
}

func (f *fakeRealtimePublisher) Publish(ctx context.Context, publication platformrealtime.Publication) error {
	f.publications = append(f.publications, publication)
	return f.err
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	return taskqueue.EnqueueResult{ID: "task-id", Queue: task.Queue, Type: task.Type}, nil
}

func TestInitReturnsTaskDicts(t *testing.T) {
	got, appErr := NewService(&fakeRepository{}).Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init returned error: %v", appErr)
	}
	if got.Dict.NotificationTargetTypeArr[0].Value != enum.NotificationTargetAll || got.Dict.NotificationTaskStatusArr[3].Value != enum.NotificationTaskStatusFailed {
		t.Fatalf("unexpected task dict: %#v", got.Dict)
	}
	if got.Dict.PlatformArr[0].Value != enum.PlatformAll {
		t.Fatalf("expected all platform first, got %#v", got.Dict.PlatformArr)
	}
}

func TestStatusCountUsesEnumOrder(t *testing.T) {
	repo := &fakeRepository{counts: map[int]int64{enum.NotificationTaskStatusSuccess: 3}}
	got, appErr := NewService(repo).StatusCount(context.Background(), StatusCountQuery{Title: " 测试 "})
	if appErr != nil {
		t.Fatalf("StatusCount returned error: %v", appErr)
	}
	if repo.gotCount.Title != "测试" {
		t.Fatalf("expected trimmed title, got %#v", repo.gotCount)
	}
	if len(got) != 4 || got[0].Value != enum.NotificationTaskStatusPending || got[2].Num != 3 {
		t.Fatalf("unexpected status counts: %#v", got)
	}
}

func TestListNormalizesAndMapsLabels(t *testing.T) {
	createdAt := time.Date(2026, 5, 5, 13, 0, 0, 0, time.UTC)
	sendAt := createdAt.Add(time.Hour)
	repo := &fakeRepository{rows: []Task{{
		ID: 7, Title: "发布", Content: "content", Type: enum.NotificationTypeWarning,
		Level: enum.NotificationLevelUrgent, Platform: enum.PlatformAll, TargetType: enum.NotificationTargetRoles,
		Status: enum.NotificationTaskStatusPending, TotalCount: 5, SentCount: 1, SendAt: &sendAt, ErrorMsg: "err", CreatedAt: createdAt,
	}}, total: 1}
	got, appErr := NewService(repo).List(context.Background(), ListQuery{
		CurrentPage: 1,
		PageSize:    20,
		Status:      ptrInt(enum.NotificationTaskStatusPending),
		Title:       " 发布 ",
	})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.gotList.Title != "发布" {
		t.Fatalf("expected trimmed list query, got %#v", repo.gotList)
	}
	item := got.List[0]
	if item.TypeText != "警告" || item.LevelText != "紧急" || item.PlatformText != "全平台" || item.TargetTypeText != "指定角色" || item.StatusText != "待发送" {
		t.Fatalf("unexpected list item labels: %#v", item)
	}
	if item.SendAt == nil || *item.SendAt != "2026-05-05 14:00:00" || item.ErrorMsg == nil {
		t.Fatalf("unexpected optional fields: %#v", item)
	}
}

func TestListRejectsInvalidStatus(t *testing.T) {
	_, appErr := NewService(&fakeRepository{}).List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Status: ptrInt(99)})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected invalid status bad request, got %#v", appErr)
	}
}

func TestCreateImmediateNormalizesAndEnqueuesSendTask(t *testing.T) {
	repo := &fakeRepository{targetCount: 2, createdID: 99}
	enqueuer := &fakeEnqueuer{}
	service := NewService(repo, WithEnqueuer(enqueuer), WithNow(func() time.Time {
		return time.Date(2026, 5, 5, 12, 0, 0, 0, time.Local)
	}))

	got, appErr := service.Create(context.Background(), CreateInput{
		Title: " 通知 ", TargetType: enum.NotificationTargetUsers, TargetIDs: []int64{3, 2, 2}, CreatedBy: 1,
	})
	if appErr != nil {
		t.Fatalf("Create returned error: %v", appErr)
	}
	if got.ID != 99 || !got.Queued {
		t.Fatalf("unexpected create response: %#v", got)
	}
	if repo.created.Title != "通知" || repo.created.Type != enum.NotificationTypeInfo || repo.created.Level != enum.NotificationLevelNormal || repo.created.Platform != enum.PlatformAll || repo.created.TotalCount != 2 {
		t.Fatalf("unexpected created row: %#v", repo.created)
	}
	var targetIDs []int64
	if err := json.Unmarshal([]byte(repo.created.TargetIDs), &targetIDs); err != nil {
		t.Fatalf("invalid target ids json: %v", err)
	}
	if len(targetIDs) != 2 || targetIDs[0] != 2 || targetIDs[1] != 3 {
		t.Fatalf("target ids not normalized: %#v", targetIDs)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != TypeSendTaskV1 || enqueuer.tasks[0].Queue != taskqueue.QueueDefault {
		t.Fatalf("expected send task enqueue, got %#v", enqueuer.tasks)
	}
}

func TestCreateScheduledDoesNotEnqueueImmediately(t *testing.T) {
	repo := &fakeRepository{targetCount: 10, createdID: 100}
	enqueuer := &fakeEnqueuer{}
	service := NewService(repo, WithEnqueuer(enqueuer), WithNow(func() time.Time {
		return time.Date(2026, 5, 5, 12, 0, 0, 0, time.Local)
	}))

	got, appErr := service.Create(context.Background(), CreateInput{
		Title: "定时", TargetType: enum.NotificationTargetAll, TargetIDs: []int64{1, 2}, SendAt: "2026-05-05 13:00:00", CreatedBy: 1,
	})
	if appErr != nil {
		t.Fatalf("Create scheduled returned error: %v", appErr)
	}
	if got.Queued || len(enqueuer.tasks) != 0 || repo.created.SendAt == nil {
		t.Fatalf("expected scheduled create without enqueue, got response=%#v tasks=%#v row=%#v", got, enqueuer.tasks, repo.created)
	}
	if repo.created.TargetIDs != "[]" {
		t.Fatalf("target all should clear ids, got %q", repo.created.TargetIDs)
	}
}

func TestCreateRejectsMissingTargets(t *testing.T) {
	_, appErr := NewService(&fakeRepository{}).Create(context.Background(), CreateInput{
		Title: "通知", TargetType: enum.NotificationTargetRoles, CreatedBy: 1,
	})
	if appErr == nil || appErr.Message != "请选择通知目标" {
		t.Fatalf("expected missing targets error, got %#v", appErr)
	}
}

func TestCreateReturnsExplicitQueueErrorAfterDBCreate(t *testing.T) {
	service := NewService(&fakeRepository{targetCount: 1, createdID: 101}, WithEnqueuer(&fakeEnqueuer{err: errors.New("redis down")}))
	got, appErr := service.Create(context.Background(), CreateInput{Title: "通知", TargetType: enum.NotificationTargetAll, CreatedBy: 1})
	if got != nil || appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected enqueue failure, got result=%#v err=%#v", got, appErr)
	}
}

func TestCancelOnlyPending(t *testing.T) {
	repo := &fakeRepository{getRow: &Task{ID: 1, Status: enum.NotificationTaskStatusSending}, cancelAffected: 1}
	appErr := NewService(repo).Cancel(context.Background(), 1)
	if appErr == nil || appErr.Message != "只能取消待发送的通知任务" {
		t.Fatalf("expected non-pending cancel rejection, got %#v", appErr)
	}

	repo.getRow.Status = enum.NotificationTaskStatusPending
	appErr = NewService(repo).Cancel(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("expected pending cancel success, got %#v", appErr)
	}
}

func TestDispatchDueClaimsAndEnqueuesSendTasks(t *testing.T) {
	repo := &fakeRepository{dueIDs: []int64{2, 3}}
	enqueuer := &fakeEnqueuer{}
	got, err := NewService(repo, WithEnqueuer(enqueuer)).DispatchDue(context.Background(), DispatchDueInput{})
	if err != nil {
		t.Fatalf("DispatchDue returned error: %v", err)
	}
	if got.Claimed != 2 || got.Queued != 2 || repo.gotDueLimit != defaultDispatchLimit {
		t.Fatalf("unexpected dispatch result=%#v repo=%#v", got, repo)
	}
	if len(enqueuer.tasks) != 2 || enqueuer.tasks[0].Type != TypeSendTaskV1 {
		t.Fatalf("unexpected enqueued send tasks: %#v", enqueuer.tasks)
	}
}

func TestSendTaskWritesNotificationsAndMarksSuccess(t *testing.T) {
	repo := &fakeRepository{
		sendTask:      &Task{ID: 7, Title: "hello", Content: "world", Type: enum.NotificationTypeSuccess, Level: enum.NotificationLevelNormal, Link: "/n", Platform: enum.PlatformAdmin, TargetType: enum.NotificationTargetUsers, TargetIDs: `[1,2]`},
		sendClaimed:   true,
		targetUserIDs: []int64{1, 2},
	}
	got, err := NewService(repo).SendTask(context.Background(), SendTaskInput{TaskID: 7})
	if err != nil {
		t.Fatalf("SendTask returned error: %v", err)
	}
	if got.Sent != 2 || len(repo.inserted) != 2 || repo.successSent != 2 || repo.successTotal != 2 {
		t.Fatalf("unexpected send result=%#v repo=%#v", got, repo)
	}
	if repo.inserted[0].IsRead != enum.CommonNo || repo.inserted[0].IsDel != enum.CommonNo {
		t.Fatalf("unexpected notification row: %#v", repo.inserted[0])
	}
}

func TestSendTaskPublishesRealtimeNotifications(t *testing.T) {
	repo := &fakeRepository{
		sendTask:      &Task{ID: 7, Title: "hello", Content: "world", Type: enum.NotificationTypeWarning, Level: enum.NotificationLevelUrgent, Link: "/notification", Platform: enum.PlatformAdmin, TargetType: enum.NotificationTargetUsers, TargetIDs: `[1,2]`},
		sendClaimed:   true,
		targetUserIDs: []int64{1, 2},
	}
	publisher := &fakeRealtimePublisher{}
	got, err := NewService(repo, WithRealtimePublisher(publisher)).SendTask(context.Background(), SendTaskInput{TaskID: 7})
	if err != nil {
		t.Fatalf("SendTask returned error: %v", err)
	}
	if got.Sent != 2 || len(publisher.publications) != 2 {
		t.Fatalf("unexpected send result=%#v publications=%#v", got, publisher.publications)
	}
	first := publisher.publications[0]
	if first.Platform != enum.PlatformAdmin || first.UserID != 1 || first.Envelope.Type != "notification.created.v1" {
		t.Fatalf("unexpected publication target/envelope: %#v", first)
	}
	var data map[string]any
	if err := json.Unmarshal(first.Envelope.Data, &data); err != nil {
		t.Fatalf("invalid publication data: %v", err)
	}
	if data["level"] != "urgent" || data["notification_type"] != "warning" || data["title"] != "hello" {
		t.Fatalf("unexpected publication data: %#v", data)
	}
}

func TestSendTaskRealtimePublishFailureDoesNotFailDBTask(t *testing.T) {
	repo := &fakeRepository{
		sendTask:      &Task{ID: 8, Title: "hello", Type: enum.NotificationTypeInfo, Level: enum.NotificationLevelNormal, Platform: enum.PlatformAdmin, TargetType: enum.NotificationTargetUsers, TargetIDs: `[1]`},
		sendClaimed:   true,
		targetUserIDs: []int64{1},
	}
	publisher := &fakeRealtimePublisher{err: errors.New("redis down")}
	got, err := NewService(repo, WithRealtimePublisher(publisher)).SendTask(context.Background(), SendTaskInput{TaskID: 8})
	if err != nil {
		t.Fatalf("SendTask should not fail on realtime publish error, got %v", err)
	}
	if got.Sent != 1 || repo.successSent != 1 || repo.failedMsg != "" {
		t.Fatalf("expected DB task success despite realtime error, got result=%#v repo=%#v", got, repo)
	}
}

func TestSendTaskSkipsAppRealtimeUntilAppWebSocketExists(t *testing.T) {
	repo := &fakeRepository{
		sendTask:      &Task{ID: 9, Title: "app only", Type: enum.NotificationTypeInfo, Level: enum.NotificationLevelNormal, Platform: enum.PlatformApp, TargetType: enum.NotificationTargetUsers, TargetIDs: `[1]`},
		sendClaimed:   true,
		targetUserIDs: []int64{1},
	}
	publisher := &fakeRealtimePublisher{}
	got, err := NewService(repo, WithRealtimePublisher(publisher)).SendTask(context.Background(), SendTaskInput{TaskID: 9})
	if err != nil {
		t.Fatalf("SendTask returned error: %v", err)
	}
	if got.Sent != 1 || len(publisher.publications) != 0 {
		t.Fatalf("expected app task DB send without admin realtime publish, got result=%#v publications=%#v", got, publisher.publications)
	}
}

func TestSendTaskNoopsWhenAlreadyDone(t *testing.T) {
	got, err := NewService(&fakeRepository{sendTask: &Task{ID: 7, Status: enum.NotificationTaskStatusSuccess}, sendClaimed: false}).SendTask(context.Background(), SendTaskInput{TaskID: 7})
	if err != nil {
		t.Fatalf("SendTask noop returned error: %v", err)
	}
	if !got.Noop {
		t.Fatalf("expected noop result, got %#v", got)
	}
}

func ptrInt(value int) *int { return &value }
