package contextengine

import (
	"context"
	"testing"
	"time"
)

func TestReconcileStableBatchRequeuesQueuedAndExpiredProcessing(t *testing.T) {
	now := time.Now().UTC()
	repository := newMemoryIngestionRepository(memoryVersion(3, DocumentVersionQueued), memoryVersion(1, DocumentVersionProcessing), memoryVersion(2, DocumentVersionReady))
	repository.expireLease(1, now.Add(-time.Minute), 1)
	queue := &recordingTaskEnqueuer{}
	reconciler := NewDocumentIndexReconciler(repository, NewDocumentVersionEnqueuer(queue), 10, 3)
	if err := reconciler.Reconcile(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("enqueued=%d, want 2", len(queue.tasks))
	}
}

func TestReconcileFinalizesExhaustedLeaseWithoutExternalWork(t *testing.T) {
	now := time.Now().UTC()
	repository := newMemoryIngestionRepository(memoryVersion(1, DocumentVersionProcessing))
	repository.expireLease(1, now.Add(-time.Minute), 3)
	queue := &recordingTaskEnqueuer{}
	reconciler := NewDocumentIndexReconciler(repository, NewDocumentVersionEnqueuer(queue), 10, 3)
	if err := reconciler.Reconcile(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 0 || repository.state(1) != DocumentVersionFailed {
		t.Fatalf("tasks=%d state=%s", len(queue.tasks), repository.state(1))
	}
}
