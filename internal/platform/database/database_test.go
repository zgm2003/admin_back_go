package database

import (
	"testing"
	"time"

	"admin_back_go/internal/config"
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
	client, err := Open(config.MySQLConfig{
		DSN:             "user:pass@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local",
		MaxOpenConns:    7,
		MaxIdleConns:    3,
		ConnMaxLifetime: 15 * time.Minute,
	})
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
