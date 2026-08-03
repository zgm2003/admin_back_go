package contextengine

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/taskqueue"
)

type recordingTaskEnqueuer struct{ tasks []taskqueue.Task }

func (queue *recordingTaskEnqueuer) Enqueue(_ context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	queue.tasks = append(queue.tasks, task)
	return taskqueue.EnqueueResult{Type: task.Type}, nil
}

func TestDocumentIndexEnqueuerPersistsOnlyVersionIdentity(t *testing.T) {
	queue := &recordingTaskEnqueuer{}
	enqueuer := NewDocumentVersionEnqueuer(queue)
	if err := enqueuer.EnqueueDocumentVersion(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].Type != TaskContextDocumentIndexV1 {
		t.Fatalf("tasks=%+v", queue.tasks)
	}
	if string(queue.tasks[0].Payload) != `{"document_version_id":42}` {
		t.Fatalf("payload=%s", queue.tasks[0].Payload)
	}
}

func TestDocumentIndexIdempotencyKeyIncludesImmutableMySQLFacts(t *testing.T) {
	facts := DocumentIndexFacts{VersionID: 2, ProfileID: 3, SourceFactsSHA256: [32]byte{1}, ParserVersion: "parser-v1", ChunkerVersion: "chunker-v1"}
	first, err := DocumentIndexIdempotencyKey(facts)
	if err != nil {
		t.Fatal(err)
	}
	facts.ProfileID++
	second, err := DocumentIndexIdempotencyKey(facts)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("profile change must change idempotency key")
	}
}
