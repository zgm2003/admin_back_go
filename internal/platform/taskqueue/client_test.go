package taskqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/config"

	"github.com/hibiken/asynq"
)

func TestNewClientRejectsEmptyRedisAddr(t *testing.T) {
	client, err := NewClient(config.RedisConfig{}, config.QueueConfig{
		DefaultQueue:    "default",
		DefaultMaxRetry: 3,
		DefaultTimeout:  30 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected empty redis addr to be rejected")
	}
	if client != nil {
		t.Fatalf("expected nil client on error")
	}
	if !errors.Is(err, ErrRedisAddrRequired) {
		t.Fatalf("expected ErrRedisAddrRequired, got %v", err)
	}
}

func TestNewClientMapsRedisAndQueueDefaults(t *testing.T) {
	client, err := NewClient(config.RedisConfig{
		Addr:     "127.0.0.1:6379",
		Password: "secret",
		DB:       0,
	}, config.QueueConfig{
		RedisDB:         3,
		DefaultQueue:    "default",
		DefaultMaxRetry: 3,
		DefaultTimeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	defer client.Close()

	if client.redisOpt.Addr != "127.0.0.1:6379" {
		t.Fatalf("expected redis addr 127.0.0.1:6379, got %q", client.redisOpt.Addr)
	}
	if client.redisOpt.Password != "secret" {
		t.Fatalf("expected redis password secret, got %q", client.redisOpt.Password)
	}
	if client.redisOpt.DB != 3 {
		t.Fatalf("expected queue redis db 3, got %d", client.redisOpt.DB)
	}
	if client.defaultQueue != "default" || client.defaultMaxRetry != 3 || client.defaultTimeout != 30*time.Second {
		t.Fatalf("unexpected queue defaults: %#v", client)
	}
}

func TestRedisConnOptUsesQueueRedisDB(t *testing.T) {
	opt, err := RedisConnOpt(config.RedisConfig{
		Addr:     "127.0.0.1:6379",
		Password: "secret",
		DB:       0,
	}, config.QueueConfig{RedisDB: 4})
	if err != nil {
		t.Fatalf("RedisConnOpt returned error: %v", err)
	}
	if opt.Addr != "127.0.0.1:6379" || opt.Password != "secret" || opt.DB != 4 {
		t.Fatalf("redis option mismatch: %#v", opt)
	}
}

func TestNormalizeTaskUsesConfiguredDefaults(t *testing.T) {
	client := &Client{
		defaultQueue:    "default",
		defaultMaxRetry: 3,
		defaultTimeout:  30 * time.Second,
	}

	task, opts, err := client.normalize(Task{
		Type:    "system:no-op:v1",
		Payload: []byte(`{"message":"ok"}`),
	})
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if task.Type() != "system:no-op:v1" {
		t.Fatalf("unexpected task type %q", task.Type())
	}
	if string(task.Payload()) != `{"message":"ok"}` {
		t.Fatalf("unexpected payload %q", string(task.Payload()))
	}

	assertOption(t, opts, asynq.Queue("default"))
	assertOption(t, opts, asynq.MaxRetry(3))
	assertOption(t, opts, asynq.Timeout(30*time.Second))
}

func TestNormalizeTaskAllowsExplicitQueueRetryTimeoutAndUniqueTTL(t *testing.T) {
	client := &Client{
		defaultQueue:    "default",
		defaultMaxRetry: 3,
		defaultTimeout:  30 * time.Second,
	}

	_, opts, err := client.normalize(Task{
		Type:      "system:no-op:v1",
		Payload:   []byte(`{}`),
		Queue:     "critical",
		MaxRetry:  7,
		Timeout:   15 * time.Second,
		UniqueTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	assertOption(t, opts, asynq.Queue("critical"))
	assertOption(t, opts, asynq.MaxRetry(7))
	assertOption(t, opts, asynq.Timeout(15*time.Second))
	assertOption(t, opts, asynq.Unique(time.Minute))
}

func TestNormalizeTaskRejectsMissingType(t *testing.T) {
	client := &Client{defaultQueue: "default"}

	_, _, err := client.normalize(Task{Payload: []byte(`{}`)})
	if !errors.Is(err, ErrTaskTypeRequired) {
		t.Fatalf("expected ErrTaskTypeRequired, got %v", err)
	}
}

func TestEnqueueRejectsNilClient(t *testing.T) {
	var client *Client

	_, err := client.Enqueue(context.Background(), Task{Type: "system:no-op:v1"})
	if !errors.Is(err, ErrClientNotReady) {
		t.Fatalf("expected ErrClientNotReady, got %v", err)
	}
}

func assertOption(t *testing.T, opts []asynq.Option, want asynq.Option) {
	t.Helper()
	for _, opt := range opts {
		if opt.Type() == want.Type() && opt.Value() == want.Value() {
			return
		}
	}
	t.Fatalf("expected option %v=%v in %#v", want.Type(), want.Value(), opts)
}
