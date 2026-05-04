package taskqueue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"admin_back_go/internal/config"

	"github.com/hibiken/asynq"
)

func TestQueueWeightsDropDisabledQueues(t *testing.T) {
	queues := queueWeights(config.QueueConfig{
		CriticalWeight: 6,
		DefaultWeight:  3,
		LowWeight:      0,
	})

	if len(queues) != 2 {
		t.Fatalf("expected two enabled queues, got %#v", queues)
	}
	if queues[QueueCritical] != 6 || queues[QueueDefault] != 3 {
		t.Fatalf("unexpected queue weights: %#v", queues)
	}
	if _, ok := queues[QueueLow]; ok {
		t.Fatalf("expected low queue to be disabled when weight is zero")
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

func TestNewServerRejectsNoEnabledQueues(t *testing.T) {
	server, err := NewServer(config.RedisConfig{Addr: "127.0.0.1:6379"}, config.QueueConfig{
		CriticalWeight: 0,
		DefaultWeight:  0,
		LowWeight:      0,
	})
	if err == nil {
		t.Fatalf("expected no enabled queues to be rejected")
	}
	if server != nil {
		t.Fatalf("expected nil server on error")
	}
	if !errors.Is(err, ErrQueueWeightRequired) {
		t.Fatalf("expected ErrQueueWeightRequired, got %v", err)
	}
}

func TestServerStartRejectsNilMux(t *testing.T) {
	server, err := NewServer(config.RedisConfig{Addr: "127.0.0.1:6379"}, config.QueueConfig{
		DefaultWeight: 1,
	})
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
