package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/telemetry"

	"github.com/hibiken/asynq"
)

func TestNormalizeUsesRegisteredTaskPolicy(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Type:      "widget:run:v1",
		Queue:     QueueLow,
		Timeout:   15 * time.Second,
		MaxRetry:  7,
		UniqueTTL: time.Minute,
		Decode:    func([]byte) (any, *apperror.Error) { return nil, nil },
		Handle:    func(context.Context, any) *apperror.Error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	client := &Client{registry: registry}

	_, opts, err := client.normalize(Task{Type: "widget:run:v1", Payload: []byte(`{"id":7}`)})
	if err != nil {
		t.Fatal(err)
	}
	assertOption(t, opts, asynq.Queue(QueueLow))
	assertOption(t, opts, asynq.MaxRetry(7))
	assertOption(t, opts, asynq.Timeout(15*time.Second))
	assertOption(t, opts, asynq.Unique(time.Minute))
}

func TestNormalizeRejectsUnregisteredTaskAndProducerPolicyOverrides(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Type:     "widget:run:v1",
		Queue:    QueueDefault,
		Timeout:  time.Minute,
		MaxRetry: 3,
		Decode:   func([]byte) (any, *apperror.Error) { return nil, nil },
		Handle:   func(context.Context, any) *apperror.Error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	client := &Client{registry: registry}

	if _, _, err := client.normalize(Task{Type: "widget:missing:v1"}); !errors.Is(err, ErrTaskTypeNotRegistered) {
		t.Fatalf("expected unregistered type error, got %v", err)
	}
	if _, _, err := client.normalize(Task{Type: "widget:run:v1", Queue: QueueCritical}); !errors.Is(err, ErrTaskPolicyOverride) {
		t.Fatalf("expected policy override rejection, got %v", err)
	}
}

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

func TestNewClientRequiresTaskRegistry(t *testing.T) {
	client, err := NewClient(
		config.RedisConfig{Addr: "127.0.0.1:6379"},
		config.QueueConfig{RedisDB: 3},
	)
	if client != nil {
		_ = client.Close()
		t.Fatal("expected nil client without registry")
	}
	if !errors.Is(err, ErrRegistryRequired) {
		t.Fatalf("expected ErrRegistryRequired, got %v", err)
	}
}

func TestNewClientMapsRedisAndUsesRegistry(t *testing.T) {
	registry := registeredTestTask(t, "system:no-op:v1", QueueDefault, DefaultMaxRetry, DefaultTimeout, 0)
	client, err := NewClient(config.RedisConfig{
		Addr:     "127.0.0.1:6379",
		Password: "secret",
		DB:       0,
	}, config.QueueConfig{RedisDB: 3}, WithRegistry(registry))
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
	if client.registry != registry {
		t.Fatalf("expected client to retain the supplied registry")
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
		registry: registeredTestTask(t, "system:no-op:v1", QueueDefault, DefaultMaxRetry, DefaultTimeout, 0),
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

func TestNormalizePassesExplicitTaskIdentityToAsynq(t *testing.T) {
	client := &Client{registry: registeredTestTask(t, "system:no-op:v1", QueueDefault, DefaultMaxRetry, DefaultTimeout, 0)}

	_, opts, err := client.normalize(Task{ID: "context-version-42", Type: "system:no-op:v1", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	assertOption(t, opts, asynq.TaskID("context-version-42"))
}

func TestNormalizeTaskRejectsExplicitQueueRetryTimeoutAndUniqueTTL(t *testing.T) {
	client := &Client{
		registry: registeredTestTask(t, "system:no-op:v1", QueueDefault, DefaultMaxRetry, DefaultTimeout, 0),
	}

	_, _, err := client.normalize(Task{
		Type:      "system:no-op:v1",
		Payload:   []byte(`{}`),
		Queue:     "critical",
		MaxRetry:  7,
		Timeout:   15 * time.Second,
		UniqueTTL: time.Minute,
	})
	if !errors.Is(err, ErrTaskPolicyOverride) {
		t.Fatalf("expected ErrTaskPolicyOverride, got %v", err)
	}
}

func TestNormalizeTaskRejectsMissingType(t *testing.T) {
	client := &Client{registry: registeredTestTask(t, "system:no-op:v1", QueueDefault, DefaultMaxRetry, DefaultTimeout, 0)}

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
		registry: registeredTestTask(t, "mail:send:v1", QueueCritical, DefaultMaxRetry, DefaultTimeout, 0),
		recorder: recorder,
		enqueue: func(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
			return &asynq.TaskInfo{ID: "task-unique", Queue: QueueCritical, Type: "mail:send:v1"}, nil
		},
	}

	result, err := client.Enqueue(context.Background(), Task{
		Type:    "mail:send:v1",
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

func registeredTestTask(t *testing.T, taskType string, queue string, maxRetry int, timeout time.Duration, uniqueTTL time.Duration) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Type:      taskType,
		Queue:     queue,
		Timeout:   timeout,
		MaxRetry:  maxRetry,
		UniqueTTL: uniqueTTL,
		Decode:    func([]byte) (any, *apperror.Error) { return nil, nil },
		Handle:    func(context.Context, any) *apperror.Error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	return registry
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
