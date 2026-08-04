package realtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPersistedEventEnvelopeRejectsDurabilityDrift(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	eventID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatalf("create event id: %v", err)
	}
	payload, err := DefaultRegistry().EncodePayload(TypeNotificationCreatedV1, NotificationCreatedPayload{
		TaskID: 1, Title: "title", Content: "body", Link: "/", Level: "normal", NotificationType: "info",
	})
	if err != nil {
		t.Fatalf("encode notification payload: %v", err)
	}
	event := Event{
		Sequence: 1, EventID: eventID, EventType: TypeNotificationCreatedV1,
		Durability: string(infrarealtime.Ephemeral), PayloadJSON: string(payload), OccurredAt: now,
	}
	if _, err := event.Envelope(DefaultRegistry()); !errors.Is(err, ErrEventDurabilityInvalid) {
		t.Fatalf("persisted durability drift was accepted: %v", err)
	}
}

func TestResumeRequiresAuthoritativeResyncWhenBacklogExceedsLimit(t *testing.T) {
	sqlDB, mock, db := newRealtimeRepositoryMockDB(t)
	defer sqlDB.Close()
	repository := &GormRepository{db: db, registry: DefaultRegistry()}
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(sequence\\), 0\\) AS value FROM `realtime_events`").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(3))
	mock.ExpectQuery("SELECT \\* FROM `realtime_event_retention_watermarks`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery("SELECT \\* FROM `realtime_events`.*ORDER BY sequence asc LIMIT \\?").
		WithArgs(TargetTypeUser, "7", infrarealtime.Durable, uint64(0), 3).
		WillReturnRows(sqlmock.NewRows([]string{
			"sequence", "event_id", "event_type", "target_type", "target_id", "durability", "payload_json", "occurred_at",
		}).
			AddRow(1, "01J00000000000000000000001", TypeNotificationCreatedV1, TargetTypeUser, "7", infrarealtime.Durable, `{}`, now).
			AddRow(2, "01J00000000000000000000002", TypeNotificationCreatedV1, TargetTypeUser, "7", infrarealtime.Durable, `{}`, now).
			AddRow(3, "01J00000000000000000000003", TypeNotificationCreatedV1, TargetTypeUser, "7", infrarealtime.Durable, `{}`, now))

	result, err := repository.ResumeUser(context.Background(), ResumeQuery{UserID: 7, AfterSequence: 0, Limit: 2, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ResyncRequired || len(result.Events) != 0 || result.LatestSequence != 3 {
		t.Fatalf("overflow resume=%#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeReturnsOrderedDurableEventsWithMonotonicCursors(t *testing.T) {
	repository := realtimeTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	first, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 7, RequestID: "request-1", OccurredAt: now,
		Payload: NotificationCreatedPayload{TaskID: 1, Title: "one", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	})
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	second, err := repository.Append(ctx, AppendInput{
		Type: TypeAIResponseCompletedV1, UserID: 7, RequestID: "request-2", OccurredAt: now.Add(time.Second),
		Payload: AIResponseCompletedPayload{ConversationID: 3, RequestID: "request-2", AssistantMessageID: 11},
	})
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}
	if _, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 8, RequestID: "other-user", OccurredAt: now.Add(2 * time.Second),
		Payload: NotificationCreatedPayload{TaskID: 2, Title: "other", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	}); err != nil {
		t.Fatalf("append other user event: %v", err)
	}

	result, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: 0, Limit: 500, Now: now})
	if err != nil {
		t.Fatalf("ResumeUser returned error: %v", err)
	}
	if result.ResyncRequired || len(result.Events) != 2 {
		t.Fatalf("unexpected resume result: %#v", result)
	}
	if result.Events[0].Sequence != first.Sequence || result.Events[1].Sequence != second.Sequence || result.Events[0].Sequence >= result.Events[1].Sequence {
		t.Fatalf("events are not ordered by monotonic cursor: %#v", result.Events)
	}
	for _, event := range result.Events {
		envelope, err := event.Envelope(DefaultRegistry())
		if err != nil {
			t.Fatalf("event envelope: %v", err)
		}
		if envelope.Durability != infrarealtime.Durable || envelope.Sequence != event.Sequence {
			t.Fatalf("unexpected durable envelope: %#v", envelope)
		}
	}

	afterFirst, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: first.Sequence, Limit: 500, Now: now})
	if err != nil || len(afterFirst.Events) != 1 || afterFirst.Events[0].Sequence != second.Sequence {
		t.Fatalf("resume after first=%#v err=%v", afterFirst, err)
	}
	future, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: second.Sequence + 100, Limit: 500, Now: now})
	if err != nil || future.ResyncRequired || len(future.Events) != 0 {
		t.Fatalf("future cursor must remain empty without guessing a resync: result=%#v err=%v", future, err)
	}
}

func TestResumeDuplicateEventIDIsRejected(t *testing.T) {
	repository := realtimeTestRepository(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	eventID, err := infrarealtime.NewEventID(now)
	if err != nil {
		t.Fatalf("NewEventID returned error: %v", err)
	}
	input := AppendInput{
		EventID: eventID, Type: TypeAIResponseCompletedV1, UserID: 7, RequestID: "request-duplicate", OccurredAt: now,
		Payload: AIResponseCompletedPayload{ConversationID: 3, RequestID: "request-duplicate", AssistantMessageID: 11},
	}
	if _, err := repository.Append(context.Background(), input); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if _, err := repository.Append(context.Background(), input); !errors.Is(err, ErrDuplicateEventID) {
		t.Fatalf("expected duplicate event ID error, got %v", err)
	}
}

func TestAppendAssignsConfirmedSevenDayRetentionAndRequestIDLimit(t *testing.T) {
	repository := realtimeTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	requestID := strings.Repeat("界", 128)
	event, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 7, RequestID: requestID, OccurredAt: now,
		Payload: NotificationCreatedPayload{TaskID: 1, Title: "retained", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	})
	if err != nil {
		t.Fatalf("append 128-character request ID: %v", err)
	}
	if event.ExpiresAt == nil || !event.ExpiresAt.Equal(now.Add(DurableEventRetention)) {
		t.Fatalf("unexpected retention expiry: %#v", event.ExpiresAt)
	}
	if _, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 7, RequestID: strings.Repeat("界", 129), OccurredAt: now,
		Payload: NotificationCreatedPayload{TaskID: 2, Title: "invalid", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	}); !errors.Is(err, ErrAppendInputInvalid) {
		t.Fatalf("129-character request ID error=%v", err)
	}
}

func TestCleanupAdvancesPerUserWatermarkAndResumeUsesIt(t *testing.T) {
	repository := realtimeTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	expired, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 7, RequestID: "expired", OccurredAt: now.Add(-DurableEventRetention - time.Second),
		Payload: NotificationCreatedPayload{TaskID: 1, Title: "expired", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	})
	if err != nil {
		t.Fatalf("append expired event: %v", err)
	}
	active, err := repository.Append(ctx, AppendInput{
		Type: TypeAIResponseCompletedV1, UserID: 7, RequestID: "active", OccurredAt: now,
		Payload: AIResponseCompletedPayload{ConversationID: 3, RequestID: "active", AssistantMessageID: 11},
	})
	if err != nil {
		t.Fatalf("append active event: %v", err)
	}
	if _, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 8, RequestID: "other-expired", OccurredAt: now.Add(-DurableEventRetention - time.Second),
		Payload: NotificationCreatedPayload{TaskID: 2, Title: "other", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	}); err != nil {
		t.Fatalf("append other user event: %v", err)
	}

	cleanup, err := repository.CleanupExpired(ctx, now, 100)
	if err != nil {
		t.Fatalf("cleanup expired events: %v", err)
	}
	if cleanup.Deleted != 2 || cleanup.Targets != 2 {
		t.Fatalf("unexpected cleanup result: %#v", cleanup)
	}

	resync, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: expired.Sequence - 1, Limit: 500, Now: now})
	if err != nil || !resync.ResyncRequired || len(resync.Events) != 0 || resync.LatestSequence != active.Sequence {
		t.Fatalf("watermark resync=%#v err=%v", resync, err)
	}
	resumed, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: expired.Sequence, Limit: 500, Now: now})
	if err != nil || resumed.ResyncRequired || len(resumed.Events) != 1 || resumed.Events[0].Sequence != active.Sequence {
		t.Fatalf("resume at watermark=%#v err=%v", resumed, err)
	}
	future, err := repository.ResumeUser(ctx, ResumeQuery{UserID: 7, AfterSequence: active.Sequence + 100, Limit: 500, Now: now})
	if err != nil || future.ResyncRequired || len(future.Events) != 0 || future.LatestSequence != active.Sequence {
		t.Fatalf("future cursor must not invent a resync: %#v err=%v", future, err)
	}
}

func TestCleanupRollbackPreservesEventAndWatermarkTogether(t *testing.T) {
	repository := realtimeTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	event, err := repository.Append(ctx, AppendInput{
		Type: TypeNotificationCreatedV1, UserID: 7, RequestID: "rollback", OccurredAt: now.Add(-DurableEventRetention - time.Second),
		Payload: NotificationCreatedPayload{TaskID: 1, Title: "rollback", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
	})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := repository.db.Exec(`CREATE TRIGGER reject_realtime_delete BEFORE DELETE ON realtime_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='reject delete'`).Error; err != nil {
		t.Fatalf("create delete trigger: %v", err)
	}
	if _, err := repository.CleanupExpired(ctx, now, 100); err == nil {
		t.Fatal("cleanup unexpectedly succeeded")
	}
	var eventCount int64
	if err := repository.db.Model(&Event{}).Where("sequence = ?", event.Sequence).Count(&eventCount).Error; err != nil || eventCount != 1 {
		t.Fatalf("event rollback count=%d err=%v", eventCount, err)
	}
	var watermarkCount int64
	if err := repository.db.Model(&RetentionWatermark{}).Where("target_type = ? AND target_id = ?", TargetTypeUser, "7").Count(&watermarkCount).Error; err != nil || watermarkCount != 0 {
		t.Fatalf("watermark rollback count=%d err=%v", watermarkCount, err)
	}
}

type failingRealtimePublisher struct{ calls int }

func (p *failingRealtimePublisher) Publish(context.Context, infrarealtime.Publication) error {
	p.calls++
	return errors.New("redis unavailable")
}

func TestResumeSurvivesRedisOutageAfterDurableCommit(t *testing.T) {
	repository := realtimeTestRepository(t)
	publisher := &failingRealtimePublisher{}
	sink := NewDurableEventSink(repository, publisher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	var persisted *Event
	err := repository.db.Transaction(func(tx *gorm.DB) error {
		var err error
		persisted, err = sink.AppendTx(context.Background(), tx, AppendInput{
			Type: TypeNotificationCreatedV1, UserID: 7, RequestID: "redis-outage", OccurredAt: now,
			Payload: NotificationCreatedPayload{TaskID: 1, Title: "offline", Content: "body", Link: "/", Level: "normal", NotificationType: "info"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("commit durable event: %v", err)
	}
	sink.PublishBestEffort(context.Background(), persisted)
	if publisher.calls != 1 {
		t.Fatalf("publisher calls=%d", publisher.calls)
	}
	result, err := repository.ResumeUser(context.Background(), ResumeQuery{UserID: 7, AfterSequence: 0, Now: now})
	if err != nil || len(result.Events) != 1 || result.Events[0].EventID != persisted.EventID {
		t.Fatalf("durable resume after Redis outage=%#v err=%v", result, err)
	}
}

func realtimeTestRepository(t *testing.T) *GormRepository {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for realtime repository tests")
	}
	base, err := database.Open(config.MySQLConfig{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open base MySQL: %v", err)
	}
	databaseName := fmt.Sprintf("p05_realtime_%d", time.Now().UnixNano())
	if err := base.Gorm.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci").Error; err != nil {
		_ = base.Close()
		t.Fatalf("create realtime test database: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("parse MySQL DSN: %v", err)
	}
	parsed.DBName = databaseName
	client, err := database.Open(config.MySQLConfig{DSN: parsed.FormatDSN(), MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("open realtime test database: %v", err)
	}
	if err := client.Gorm.Exec("CREATE TABLE realtime_events LIKE admin.realtime_events").Error; err != nil {
		_ = client.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("create realtime events table: %v", err)
	}
	if err := client.Gorm.Exec("ALTER TABLE realtime_events MODIFY request_id VARCHAR(128) NULL, MODIFY expires_at DATETIME(6) NOT NULL").Error; err != nil {
		t.Fatalf("align realtime event contract: %v", err)
	}
	if err := client.Gorm.Exec(`CREATE TABLE realtime_event_retention_watermarks (
target_type VARCHAR(16) NOT NULL,
target_id VARCHAR(64) NOT NULL,
deleted_through_sequence BIGINT UNSIGNED NOT NULL DEFAULT 0,
updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
PRIMARY KEY (target_type,target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`).Error; err != nil {
		t.Fatalf("create realtime retention watermark table: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
	})
	return NewGormRepository(client, DefaultRegistry())
}

func newRealtimeRepositoryMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *gorm.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return sqlDB, mock, db
}
