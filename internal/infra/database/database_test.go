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
