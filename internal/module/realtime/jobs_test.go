package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/infra/taskqueue"
)

type fakeEventCleaner struct {
	results []CleanupResult
	err     error
	calls   int
	now     time.Time
	limit   int
}

func (f *fakeEventCleaner) CleanupExpired(_ context.Context, now time.Time, limit int) (CleanupResult, error) {
	f.calls++
	f.now = now
	f.limit = limit
	if f.err != nil {
		return CleanupResult{}, f.err
	}
	if len(f.results) == 0 {
		return CleanupResult{}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

func TestRetentionServiceDrainsBoundedAtomicBatches(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 15, 0, 0, time.UTC)
	cleaner := &fakeEventCleaner{results: []CleanupResult{
		{Deleted: DefaultCleanupBatchSize, Targets: 2},
		{Deleted: 3, Targets: 1},
	}}
	service := NewRetentionService(cleaner)
	service.now = func() time.Time { return now }
	if err := service.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("cleanup expired events: %v", err)
	}
	if cleaner.calls != 2 || !cleaner.now.Equal(now) || cleaner.limit != DefaultCleanupBatchSize {
		t.Fatalf("unexpected cleanup calls=%d now=%v limit=%d", cleaner.calls, cleaner.now, cleaner.limit)
	}

	wantErr := errors.New("database unavailable")
	if err := NewRetentionService(&fakeEventCleaner{err: wantErr}).CleanupExpired(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("cleanup error=%v", err)
	}
}

func TestRealtimeCleanupTaskIsVersionedRegisteredAndStrict(t *testing.T) {
	cleaner := &fakeEventCleaner{}
	service := NewRetentionService(cleaner)
	registry := taskqueue.NewRegistry()
	if err := RegisterTaskDefinitions(registry, service, nil); err != nil {
		t.Fatalf("register realtime cleanup task: %v", err)
	}
	task, err := NewCleanupExpiredTask()
	if err != nil {
		t.Fatalf("build realtime cleanup task: %v", err)
	}
	registered, policy, err := registry.Task(task.Type, task.Payload)
	if err != nil {
		t.Fatalf("resolve registered task: %v", err)
	}
	if registered.Type != TypeCleanupExpiredV1 || policy.Queue != taskqueue.QueueLow || policy.Timeout != time.Minute || policy.UniqueTTL != 23*time.Hour {
		t.Fatalf("unexpected cleanup task/policy: %#v %#v", registered, policy)
	}
	if err := registry.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle cleanup task: %v", err)
	}
	if cleaner.calls != 1 {
		t.Fatalf("cleanup calls=%d", cleaner.calls)
	}
	if err := registry.Handle(context.Background(), taskqueue.Task{Type: TypeCleanupExpiredV1, Payload: []byte(`{"fallback":true}`)}); err == nil {
		t.Fatal("cleanup task accepted an undocumented payload field")
	}
}
