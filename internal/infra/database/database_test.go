package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestOpenRejectsEmptyDSN(t *testing.T) {
	client, err := Open(config.MySQLConfig{})
	if err == nil {
		if client != nil {
			_ = client.Close()
		}
		t.Fatalf("expected empty dsn to be rejected")
	}
	if client != nil {
		t.Fatalf("expected nil client on error")
	}
}

func TestOpenCreatesGormClientWithoutLiveMySQL(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	client, err := Open(config.MySQLConfig{
		DSN:             "user:pass@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local",
		MaxOpenConns:    7,
		MaxIdleConns:    3,
		ConnMaxLifetime: 15 * time.Minute,
	}, WithTelemetry(recorder))
	if err != nil {
		t.Fatalf("expected open without live mysql ping, got error: %v", err)
	}
	defer client.Close()

	if client.Gorm == nil {
		t.Fatalf("expected gorm handle")
	}
	if client.SQL == nil {
		t.Fatalf("expected sql handle")
	}
	if got := client.SQL.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("expected max open connections 7, got %d", got)
	}
}

func TestTelemetryLoggerRecordsOnlyOperationTableDurationAndHashedSlowDigest(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	logger := newTelemetryLogger(gormlogger.Discard, recorder, 10*time.Millisecond)
	query := "SELECT * FROM `users` WHERE email = 'private@example.com'"
	logger.Trace(context.Background(), time.Now().Add(-20*time.Millisecond), func() (string, int64) {
		return query, 1
	}, errors.New("driver error containing private@example.com"))

	events := recorder.Events()
	if len(events) != 3 {
		t.Fatalf("expected query count, duration, and slow query, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["db.operation"] != "SELECT" || event.Attributes["db.table"] != "users" {
			t.Fatalf("database classification missing: %+v", event)
		}
	}
	text := strings.ToLower(fmt.Sprint(events))
	if strings.Contains(text, "private@example.com") || strings.Contains(text, "select *") {
		t.Fatalf("SQL or bind leaked into telemetry: %s", text)
	}
	digest, ok := events[2].Attributes["db.slow_digest"].(string)
	if !ok || len(digest) != 16 {
		t.Fatalf("slow digest was not bounded: %+v", events[2])
	}
}

func TestTelemetryLoggerTreatsRecordNotFoundAsExpectedEmptyResultForDelegate(t *testing.T) {
	delegate := &traceCaptureLogger{}
	logger := newTelemetryLogger(delegate, telemetry.Noop(), time.Second)

	logger.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM `jobs` LIMIT 1", 0
	}, fmt.Errorf("empty queue: %w", gorm.ErrRecordNotFound))

	if delegate.traceCalls != 1 {
		t.Fatalf("expected one delegated trace, got %d", delegate.traceCalls)
	}
	if delegate.traceErr != nil {
		t.Fatalf("expected record-not-found to reach the delegate as a successful empty result, got %v", delegate.traceErr)
	}

	driverErr := errors.New("driver unavailable")
	logger.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 0
	}, driverErr)
	if !errors.Is(delegate.traceErr, driverErr) {
		t.Fatalf("expected real database error to remain visible, got %v", delegate.traceErr)
	}
}

type traceCaptureLogger struct {
	traceCalls int
	traceErr   error
}

func (logger *traceCaptureLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface {
	return logger
}

func (logger *traceCaptureLogger) Info(context.Context, string, ...interface{}) {}

func (logger *traceCaptureLogger) Warn(context.Context, string, ...interface{}) {}

func (logger *traceCaptureLogger) Error(context.Context, string, ...interface{}) {}

func (logger *traceCaptureLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), traceErr error) {
	logger.traceCalls++
	logger.traceErr = traceErr
}

var _ gormlogger.Interface = (*traceCaptureLogger)(nil)
