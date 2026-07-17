package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"github.com/hibiken/asynq"
)

func TestNewClientRejectsEmptyRedisAddr(t *testing.T) {
	client, err := NewClient(config.RedisConfig{}, config.QueueConfig{})
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
	}, config.QueueConfig{RedisDB: 3})
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
	if client.defaultQueue != QueueDefault || client.defaultMaxRetry != DefaultMaxRetry || client.defaultTimeout != DefaultTimeout {
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

func TestNormalizeTaskUsesCodeOwnedDefaults(t *testing.T) {
	client := &Client{
		defaultQueue:    QueueDefault,
		defaultMaxRetry: DefaultMaxRetry,
		defaultTimeout:  DefaultTimeout,
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

	assertOption(t, opts, asynq.Queue(QueueDefault))
	assertOption(t, opts, asynq.MaxRetry(DefaultMaxRetry))
	assertOption(t, opts, asynq.Timeout(DefaultTimeout))
}

func TestNormalizeTaskAllowsExplicitQueueRetryTimeoutAndUniqueTTL(t *testing.T) {
	client := &Client{
		defaultQueue:    QueueDefault,
		defaultMaxRetry: DefaultMaxRetry,
		defaultTimeout:  DefaultTimeout,
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

func TestEnqueueRecordsBoundedTelemetryWithoutPayload(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	client := &Client{
		defaultQueue:    QueueDefault,
		defaultMaxRetry: DefaultMaxRetry,
		defaultTimeout:  DefaultTimeout,
		recorder:        recorder,
		enqueue: func(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
			return &asynq.TaskInfo{ID: "task-unique", Queue: QueueCritical, Type: "mail:send:v1"}, nil
		},
	}

	result, err := client.Enqueue(context.Background(), Task{
		Type:    "mail:send:v1",
		Queue:   QueueCritical,
		Payload: []byte(`{"authorization":"private"}`),
	})
	if err != nil || result.Queue != QueueCritical {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected enqueue count and duration, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["queue.type"] != "mail:send:v1" || event.Attributes["queue.lane"] != QueueCritical || event.Attributes["queue.outcome"] != "enqueued" {
			t.Fatalf("queue telemetry mismatch: %+v", event)
		}
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(events)), "private") {
		t.Fatalf("queue payload leaked: %+v", events)
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
