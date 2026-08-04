package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"admin_back_go/internal/infra/taskqueue"
)

func TestProfileRebuildEnqueuerUsesImmutableProfileIdentity(t *testing.T) {
	target := uint64(1)
	profile := ContextProfile{
		ID: 7, EmbeddingProviderModelID: 11, EmbeddingDimensions: 1536,
		EmbeddingMaxInputTokens: 8191, EmbeddingTokenCounterID: "utf8_bytes_v1",
		DenseDistance: string(DenseDistanceCosine), DenseMinScore: "0.200000",
		SparseEncoder: SparseEncoderUnicodeLexicalV1, SparseEncoderVersion: SparseEncoderVersionV1,
		Status: ProfileEnabled, IndexState: ProfileIndexProvisioning, TargetIndexGeneration: &target,
	}
	queue := &recordingTaskEnqueuer{}
	enqueuer := NewProfileRebuildEnqueuer(queue)
	if err := enqueuer.EnqueueProfileRebuild(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].Type != TaskContextProfileRebuildV1 || queue.tasks[0].ID == "" {
		t.Fatalf("tasks=%+v", queue.tasks)
	}
	var payload ContextProfileRebuildV1
	if err := json.Unmarshal(queue.tasks[0].Payload, &payload); err != nil || payload.ProfileID != profile.ID {
		t.Fatalf("payload=%s error=%v", queue.tasks[0].Payload, err)
	}
}

type recordingTaskEnqueuer struct{ tasks []taskqueue.Task }

func (queue *recordingTaskEnqueuer) Enqueue(_ context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	queue.tasks = append(queue.tasks, task)
	return taskqueue.EnqueueResult{Type: task.Type}, nil
}

func TestMemoryBuildTaskIdentityIncludesCompleteSourceIdentity(t *testing.T) {
	payload := ContextMemoryBuildV1{ProfileID: 2, ProfileSHA256: sha256.Sum256([]byte("profile")), ConversationID: 3,
		FromMessageID: 4, ThroughMessageID: 5, SourceSHA256: sha256.Sum256([]byte("source")), PolicyVersion: MemoryPolicyVersionV1}
	first, err := MemoryTaskIdentity(payload)
	if err != nil {
		t.Fatal(err)
	}
	parent := uint64(9)
	payload.PreviousMemoryID = &parent
	second, err := MemoryTaskIdentity(payload)
	if err != nil || first == second {
		t.Fatalf("parent must change task identity: %q %q %v", first, second, err)
	}
}

type fixedDocumentIndexFactsLoader struct{ facts DocumentIndexFacts }

func (loader fixedDocumentIndexFactsLoader) LoadDocumentIndexFacts(context.Context, uint64) (DocumentIndexFacts, error) {
	return loader.facts, nil
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

func TestDocumentIndexProducerUsesMySQLFactsAsTaskIdentity(t *testing.T) {
	facts := DocumentIndexFacts{VersionID: 42, ProfileID: 3, SourceFactsSHA256: [32]byte{1}, ParserVersion: "1", ChunkerVersion: ChunkerVersionV1}
	want, err := DocumentIndexIdempotencyKey(facts)
	if err != nil {
		t.Fatal(err)
	}
	queue := &recordingTaskEnqueuer{}
	enqueuer := NewDocumentVersionEnqueuer(queue, fixedDocumentIndexFactsLoader{facts: facts})
	if err := enqueuer.EnqueueDocumentVersion(context.Background(), facts.VersionID); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].ID != want {
		t.Fatalf("task identity=%q, want %q", queue.tasks[0].ID, want)
	}
}
