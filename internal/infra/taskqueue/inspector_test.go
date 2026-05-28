package taskqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

func TestQueueInfoErrorMapsPublicAsynqQueueNotFound(t *testing.T) {
	got := normalizeQueueInfoError("default", asynq.ErrQueueNotFound)

	if !errors.Is(got, ErrQueueNotFound) {
		t.Fatalf("expected ErrQueueNotFound, got %v", got)
	}
}

func TestQueueInfoErrorMapsAsynqCurrentStatsQueueNotFound(t *testing.T) {
	got := normalizeQueueInfoError("default", errors.New(`NOT_FOUND: queue "default" does not exist`))

	if !errors.Is(got, ErrQueueNotFound) {
		t.Fatalf("expected ErrQueueNotFound, got %v", got)
	}
}

func TestQueueInfoErrorKeepsRedisFailuresVisible(t *testing.T) {
	redisErr := errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")
	got := normalizeQueueInfoError("default", redisErr)

	if got == nil {
		t.Fatalf("expected redis error to stay visible")
	}
	if errors.Is(got, ErrQueueNotFound) {
		t.Fatalf("did not expect redis error to be treated as queue not found: %v", got)
	}
}

func TestInspectorQueueInfoRejectsNilClient(t *testing.T) {
	var inspector *Inspector

	_, err := inspector.QueueInfo(context.Background(), "default")

	if !errors.Is(err, ErrClientNotReady) {
		t.Fatalf("expected ErrClientNotReady, got %v", err)
	}
}
