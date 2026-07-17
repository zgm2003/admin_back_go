package redisclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	redis "github.com/redis/go-redis/v9"
)

func TestOpenMapsConfigToOptions(t *testing.T) {
	client, err := Open(config.RedisConfig{
		Addr:     "127.0.0.1:6380",
		Password: "secret",
		DB:       2,
	})
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	defer client.Close()

	if client.Redis == nil {
		t.Fatalf("expected redis handle")
	}

	options := client.Redis.Options()
	if options.Addr != "127.0.0.1:6380" {
		t.Fatalf("expected addr 127.0.0.1:6380, got %q", options.Addr)
	}
	if options.Password != "secret" {
		t.Fatalf("expected password secret, got %q", options.Password)
	}
	if options.DB != 2 {
		t.Fatalf("expected db 2, got %d", options.DB)
	}
}

func TestOpenRejectsEmptyAddress(t *testing.T) {
	client, err := Open(config.RedisConfig{})
	if !errors.Is(err, ErrEmptyAddress) || client != nil {
		t.Fatalf("client=%+v err=%v", client, err)
	}
}

func TestCloseIsSafeOnNilClient(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("expected nil close to be safe, got %v", err)
	}
}

func TestTelemetryHookRecordsCommandOutcomeWithoutKeyOrArguments(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	hook := newTelemetryHook(recorder)
	wrapped := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return errors.New("redis failure for private:key")
	})
	command := redis.NewStringCmd(context.Background(), "get", "private:key")

	err := wrapped(context.Background(), command)
	if err == nil {
		t.Fatal("expected wrapped Redis error")
	}
	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected command count and duration, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["redis.operation"] != "get" || event.Attributes["outcome"] != "error" {
			t.Fatalf("Redis telemetry missing bounded outcome: %+v", event)
		}
	}
	if text := strings.ToLower(fmt.Sprint(events)); strings.Contains(text, "private:key") || strings.Contains(text, "redis failure") {
		t.Fatalf("Redis key or error text leaked: %s", text)
	}
}
