package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/taskqueue"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestTaskBuildersUseVersionedTypesWithoutDuplicatingRegistryPolicy(t *testing.T) {
	dispatchTask, err := NewDispatchDueTask(DispatchDuePayload{Limit: 25})
	if err != nil {
		t.Fatalf("NewDispatchDueTask returned error: %v", err)
	}
	if dispatchTask.Type != TypeDispatchDueV1 || dispatchTask.Queue != "" || dispatchTask.UniqueTTL != 0 {
		t.Fatalf("unexpected dispatch task: %#v", dispatchTask)
	}
	var dispatchPayload DispatchDuePayload
	if err := json.Unmarshal(dispatchTask.Payload, &dispatchPayload); err != nil {
		t.Fatalf("decode dispatch payload: %v", err)
	}
	if dispatchPayload.Limit != 25 {
		t.Fatalf("unexpected dispatch payload: %#v", dispatchPayload)
	}

	sendTask, err := NewSendTask(99)
	if err != nil {
		t.Fatalf("NewSendTask returned error: %v", err)
	}
	if sendTask.Type != TypeSendTaskV1 || sendTask.Queue != "" {
		t.Fatalf("unexpected send task: %#v", sendTask)
	}
	payload, err := DecodeSendTaskPayload(sendTask.Payload)
	if err != nil {
		t.Fatalf("DecodeSendTaskPayload returned error: %v", err)
	}
	if payload.TaskID != 99 {
		t.Fatalf("unexpected send payload: %#v", payload)
	}
}

func TestRegisterHandlersProcessesDispatchAndSendTasks(t *testing.T) {
	service := &fakeJobService{}
	mux := taskqueue.NewMux()
	RegisterHandlers(mux, service, slog.Default())

	dispatchTask, err := NewDispatchDueTask(DispatchDuePayload{Limit: 5})
	if err != nil {
		t.Fatalf("NewDispatchDueTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), dispatchTask); err != nil {
		t.Fatalf("process dispatch task: %v", err)
	}
	if service.dispatchLimit != 5 {
		t.Fatalf("expected dispatch limit 5, got %d", service.dispatchLimit)
	}

	sendTask, err := NewSendTask(8)
	if err != nil {
		t.Fatalf("NewSendTask returned error: %v", err)
	}
	if err := mux.ProcessProjectTask(context.Background(), sendTask); err != nil {
		t.Fatalf("process send task: %v", err)
	}
	if service.sendTaskID != 8 {
		t.Fatalf("expected send task id 8, got %d", service.sendTaskID)
	}
}

type fakeJobService struct {
	dispatchLimit int
	sendTaskID    int64
}

func (f *fakeJobService) DispatchDue(ctx context.Context, input DispatchDueInput) (*DispatchDueResult, error) {
	f.dispatchLimit = input.Limit
	return &DispatchDueResult{Claimed: 1, Queued: 1}, nil
}

func (f *fakeJobService) SendTask(ctx context.Context, input SendTaskInput) (*SendTaskResult, error) {
	f.sendTaskID = input.TaskID
	return &SendTaskResult{TaskID: input.TaskID, Sent: 1}, nil
}

func TestClaimNotificationWorkIsExclusiveReclaimableAndFenced(t *testing.T) {
	first, second := notificationLeaseRepositories(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	row := Task{
		Title: "lease test", Content: "content", Type: enum.NotificationTypeInfo,
		Level: enum.NotificationLevelNormal, Platform: enum.PlatformAdmin,
		TargetType: enum.NotificationTargetUsers, TargetIDs: "[1]",
		Status: enum.NotificationTaskStatusPending, SendAt: &now,
		CreatedBy: 1, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now,
	}
	id, err := first.Create(context.Background(), row)
	if err != nil {
		t.Fatalf("create notification task: %v", err)
	}

	start := make(chan struct{})
	claims := make(chan *Claim, 2)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for _, worker := range []struct {
		repository *GormRepository
		owner      string
	}{{first, "worker-a"}, {second, "worker-b"}} {
		workers.Add(1)
		go func(repository *GormRepository, owner string) {
			defer workers.Done()
			<-start
			claim, claimErr := repository.ClaimNext(context.Background(), owner, now, time.Minute)
			claims <- claim
			errs <- claimErr
		}(worker.repository, worker.owner)
	}
	close(start)
	workers.Wait()
	close(claims)
	close(errs)
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatalf("concurrent claim: %v", claimErr)
		}
	}
	var winner *Claim
	claimCount := 0
	for claim := range claims {
		if claim != nil {
			winner = claim
			claimCount++
		}
	}
	if claimCount != 1 || winner == nil || winner.Task.ID != id || winner.Token == 0 {
		t.Fatalf("expected exactly one claim, count=%d winner=%#v", claimCount, winner)
	}

	if ok, err := first.MarkSuccess(context.Background(), id, "stale-owner", winner.Token, now.Add(time.Second), 1, 1); err != nil || ok {
		t.Fatalf("stale owner terminalized work: ok=%v err=%v", ok, err)
	}
	renewedUntil := winner.LeaseExpiresAt.Add(time.Minute)
	if ok, err := first.Renew(context.Background(), id, winner.Owner, winner.Token, now.Add(time.Second), renewedUntil); err != nil || !ok {
		t.Fatalf("current notification owner could not renew: ok=%v err=%v", ok, err)
	}
	if ok, err := second.Renew(context.Background(), id, "stale-owner", winner.Token, now.Add(time.Second), renewedUntil); err != nil || ok {
		t.Fatalf("stale notification owner renewed lease: ok=%v err=%v", ok, err)
	}
	reclaimed, err := second.ClaimNext(context.Background(), "worker-c", renewedUntil.Add(time.Microsecond), time.Minute)
	if err != nil {
		t.Fatalf("reclaim expired notification task: %v", err)
	}
	if reclaimed == nil || reclaimed.Token <= winner.Token {
		t.Fatalf("expected higher fencing token after expiry: first=%#v second=%#v", winner, reclaimed)
	}
	if ok, err := first.MarkSuccess(context.Background(), id, winner.Owner, winner.Token, reclaimed.LeaseExpiresAt.Add(-time.Second), 1, 1); err != nil || ok {
		t.Fatalf("stale token terminalized reclaimed work: ok=%v err=%v", ok, err)
	}
	if ok, err := second.MarkSuccess(context.Background(), id, reclaimed.Owner, reclaimed.Token, reclaimed.LeaseExpiresAt.Add(-time.Second), 1, 1); err != nil || !ok {
		t.Fatalf("current owner could not terminalize work: ok=%v err=%v", ok, err)
	}
}

func TestDuplicateNotificationDeliveryUsesSourceTaskUserKey(t *testing.T) {
	repository, _ := notificationLeaseRepositories(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	rows := []Notification{{
		SourceTaskID: 77, UserID: 9, Title: "once", Content: "content",
		Type: enum.NotificationTypeInfo, Level: enum.NotificationLevelNormal,
		Platform: enum.PlatformAdmin, IsRead: enum.CommonNo, IsDel: enum.CommonNo,
		CreatedAt: now, UpdatedAt: now,
	}}
	if err := repository.InsertNotifications(ctx, rows); err != nil {
		t.Fatalf("first notification insert: %v", err)
	}
	duplicate := append([]Notification(nil), rows...)
	duplicate[0].ID = 0
	if err := repository.InsertNotifications(ctx, duplicate); err != nil {
		t.Fatalf("duplicate notification insert: %v", err)
	}
	var count int64
	if err := repository.db.WithContext(ctx).Model(&Notification{}).
		Where("source_task_id = ? AND user_id = ?", 77, 9).Count(&count).Error; err != nil {
		t.Fatalf("count duplicate notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one notification row, got %d", count)
	}
	if err := repository.db.WithContext(ctx).Table("realtime_events").Count(&count).Error; err != nil {
		t.Fatalf("count durable notification events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one durable notification event, got %d", count)
	}
}

func TestNotificationDeliveryRejectsUnknownEnumInsteadOfGuessingRealtimePayload(t *testing.T) {
	repository, _ := notificationLeaseRepositories(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	err := repository.InsertNotifications(ctx, []Notification{{
		SourceTaskID: 88, UserID: 9, Title: "invalid", Content: "content",
		Type: enum.NotificationTypeInfo, Level: 999,
		Platform: enum.PlatformAdmin, IsRead: enum.CommonNo, IsDel: enum.CommonNo,
		CreatedAt: now, UpdatedAt: now,
	}})
	if err == nil {
		t.Fatal("unknown notification level was guessed into a realtime payload")
	}
	var notifications int64
	if queryErr := repository.db.WithContext(ctx).Model(&Notification{}).Count(&notifications).Error; queryErr != nil {
		t.Fatalf("count notifications: %v", queryErr)
	}
	var events int64
	if queryErr := repository.db.WithContext(ctx).Table("realtime_events").Count(&events).Error; queryErr != nil {
		t.Fatalf("count realtime events: %v", queryErr)
	}
	if notifications != 0 || events != 0 {
		t.Fatalf("invalid notification escaped atomic rollback: notifications=%d events=%d", notifications, events)
	}
}

func notificationLeaseRepositories(t *testing.T) (*GormRepository, *GormRepository) {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for notification lease integration tests")
	}
	base, err := database.Open(config.MySQLConfig{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open base MySQL: %v", err)
	}
	databaseName := fmt.Sprintf("p05_notification_%d", time.Now().UnixNano())
	if err := base.Gorm.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci").Error; err != nil {
		_ = base.Close()
		t.Fatalf("create notification test database: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("parse MySQL DSN: %v", err)
	}
	parsed.DBName = databaseName
	testConfig := config.MySQLConfig{DSN: parsed.FormatDSN(), MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute}
	clientA, err := database.Open(testConfig)
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("open first notification test client: %v", err)
	}
	clientB, err := database.Open(testConfig)
	if err != nil {
		_ = clientA.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("open second notification test client: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE notification_task LIKE admin.notification_task",
		"CREATE TABLE notifications LIKE admin.notifications",
		"CREATE TABLE realtime_events LIKE admin.realtime_events",
	} {
		if err := clientA.Gorm.Exec(statement).Error; err != nil {
			_ = clientB.Close()
			_ = clientA.Close()
			_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
			_ = base.Close()
			t.Fatalf("create notification test table: %v", err)
		}
	}
	t.Cleanup(func() {
		_ = clientB.Close()
		_ = clientA.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
	})
	eventRepositoryA := modulerealtime.NewGormRepository(clientA, modulerealtime.DefaultRegistry())
	eventRepositoryB := modulerealtime.NewGormRepository(clientB, modulerealtime.DefaultRegistry())
	return &GormRepository{db: clientA.Gorm, eventSink: modulerealtime.NewDurableEventSink(eventRepositoryA, infrarealtime.NoopPublisher{}, slog.Default())},
		&GormRepository{db: clientB.Gorm, eventSink: modulerealtime.NewDurableEventSink(eventRepositoryB, infrarealtime.NoopPublisher{}, slog.Default())}
}
