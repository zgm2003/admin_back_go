package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	"admin_back_go/internal/telemetry"

	"github.com/hibiken/asynq"
)

func TestQueueWeightsUseCodeOwnedDefaults(t *testing.T) {
	queues := queueWeights()

	if len(queues) != 3 {
		t.Fatalf("expected three enabled queues, got %#v", queues)
	}
	if queues[QueueCritical] != DefaultCriticalWeight {
		t.Fatalf("unexpected critical weight: %#v", queues)
	}
	if queues[QueueDefault] != DefaultQueueWeight {
		t.Fatalf("unexpected default weight: %#v", queues)
	}
	if queues[QueueLow] != DefaultLowWeight {
		t.Fatalf("unexpected low weight: %#v", queues)
	}
}

func TestNewServerRejectsEmptyRedisAddr(t *testing.T) {
	server, err := NewServer(config.RedisConfig{}, config.QueueConfig{})
	if err == nil {
		t.Fatalf("expected empty redis addr to be rejected")
	}
	if server != nil {
		t.Fatalf("expected nil server on error")
	}
	if !errors.Is(err, ErrRedisAddrRequired) {
		t.Fatalf("expected ErrRedisAddrRequired, got %v", err)
	}
}

func TestServerStartRejectsNilMux(t *testing.T) {
	server, err := NewServer(config.RedisConfig{Addr: "127.0.0.1:6379"}, config.QueueConfig{Concurrency: 1})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	if err := server.Start(nil); !errors.Is(err, ErrHandlerRequired) {
		t.Fatalf("expected ErrHandlerRequired, got %v", err)
	}
}

func TestMuxHandleFuncPassesProjectTask(t *testing.T) {
	mux := NewMux()
	var got Task
	mux.HandleFunc("system:no-op:v1", func(ctx context.Context, task Task) error {
		got = task
		return nil
	})

	err := mux.ProcessTask(context.Background(), asynq.NewTask("system:no-op:v1", []byte(`{"message":"ok"}`)))
	if err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if got.Type != "system:no-op:v1" {
		t.Fatalf("expected task type system:no-op:v1, got %q", got.Type)
	}
	if string(got.Payload) != `{"message":"ok"}` {
		t.Fatalf("unexpected payload %q", string(got.Payload))
	}
}

func TestMuxRejectsUnknownTaskTypeVisibly(t *testing.T) {
	mux := NewMux()

	err := mux.ProcessProjectTask(context.Background(), Task{Type: "system:unknown:v1"})

	if !errors.Is(err, ErrHandlerNotRegistered) {
		t.Fatalf("expected ErrHandlerNotRegistered, got %v", err)
	}
	if !strings.Contains(err.Error(), "system:unknown:v1") {
		t.Fatalf("expected error to include task type, got %v", err)
	}
}

func TestMuxRecordsRetryExhaustionAndLeaseExpiryWithoutPayload(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	mux := NewMux(WithTelemetry(recorder))
	mux.HandleFunc("ai:generate:v1", func(context.Context, Task) error {
		return context.DeadlineExceeded
	})

	err := mux.ProcessProjectTask(context.Background(), Task{
		Type:     "ai:generate:v1",
		Payload:  []byte(`{"prompt":"private"}`),
		Queue:    QueueLow,
		Retry:    3,
		MaxRetry: 3,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected handler deadline, got %v", err)
	}
	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected handler count and duration, got %+v", events)
	}
	for _, event := range events {
		attrs := event.Attributes
		if attrs["queue.type"] != "ai:generate:v1" || attrs["queue.lane"] != QueueLow || attrs["queue.retry"] != "3" {
			t.Fatalf("queue execution attributes missing: %+v", event)
		}
		if attrs["queue.outcome"] != "error" || attrs["queue.exhausted"] != "true" {
			t.Fatalf("queue terminal classification missing: %+v", event)
		}
		if attrs["queue.lease_expired"] == "true" {
			t.Fatalf("handler deadline must not be reported as lease expiry: %+v", event)
		}
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(events)), "private") {
		t.Fatalf("queue payload leaked: %+v", events)
	}
}

func TestServerErrorHandlerRecordsRealLeaseExpiryAndExhaustion(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	server, err := NewServer(
		config.RedisConfig{Addr: "127.0.0.1:6379"},
		config.QueueConfig{Concurrency: 1},
		WithTelemetry(recorder),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server.errorHandler == nil {
		t.Fatal("queue server error handler is required")
	}
	server.errorHandler.HandleError(context.Background(), asynq.NewTask("ai:generate:v1", []byte(`{"prompt":"private"}`)), fmt.Errorf("worker heartbeat: %w", asynq.ErrLeaseExpired))

	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("expected one terminal failure event, got %+v", events)
	}
	attrs := events[0].Attributes
	if attrs["queue.type"] != "ai:generate:v1" || attrs["queue.lane"] != QueueDefault || attrs["queue.retry"] != "0" {
		t.Fatalf("queue failure metadata missing: %+v", events[0])
	}
	if attrs["queue.lease_expired"] != "true" || attrs["queue.exhausted"] != "true" || attrs["queue.outcome"] != "error" {
		t.Fatalf("real lease failure classification missing: %+v", events[0])
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(events)), "private") {
		t.Fatalf("queue failure payload leaked: %+v", events)
	}
}
