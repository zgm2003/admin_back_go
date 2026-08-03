package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/infra/contextindex"
)

type memoryRepairRepositoryStub struct {
	payloads []ContextMemoryBuildV1
	next     uint64
}

func (stub memoryRepairRepositoryStub) ListMemoryBuildPayloads(context.Context, uint64, int) ([]ContextMemoryBuildV1, uint64, error) {
	return stub.payloads, stub.next, nil
}

func TestMemoryReconcilerRequeuesAuthoritativeSourceIdentity(t *testing.T) {
	payload := ContextMemoryBuildV1{ProfileID: 2, ProfileSHA256: sha256.Sum256([]byte("profile")), ConversationID: 3,
		FromMessageID: 4, ThroughMessageID: 5, SourceSHA256: sha256.Sum256([]byte("source")), PolicyVersion: MemoryPolicyVersionV1}
	queue := &recordingTaskEnqueuer{}
	reconciler := &DocumentIndexReconciler{batchSize: 10, memoryRepository: memoryRepairRepositoryStub{payloads: []ContextMemoryBuildV1{payload}, next: 7}, memoryEnqueuer: NewMemoryBuildEnqueuer(queue)}
	worked, err := reconciler.reconcileMemories(context.Background())
	if err != nil || !worked || reconciler.memoryAfterConversationID != 7 {
		t.Fatalf("worked=%v cursor=%d err=%v", worked, reconciler.memoryAfterConversationID, err)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].Type != TaskContextMemoryBuildV1 {
		t.Fatalf("tasks=%#v", queue.tasks)
	}
}

type conversationRepairRepositoryStub struct {
	afterRunIDs []uint64
	payload     ContextConversationIndexV1
}

func (repository *conversationRepairRepositoryStub) ListConversationIndexPayloads(_ context.Context, afterRunID uint64, _ int) ([]ContextConversationIndexV1, uint64, error) {
	repository.afterRunIDs = append(repository.afterRunIDs, afterRunID)
	if afterRunID == 0 {
		return []ContextConversationIndexV1{repository.payload}, 42, nil
	}
	return nil, 0, nil
}

func TestConversationTurnBackfillRestartsBoundedRunScan(t *testing.T) {
	queue := &recordingTaskEnqueuer{}
	repository := &conversationRepairRepositoryStub{payload: ContextConversationIndexV1{
		ProfileID: 7, ConversationID: 11, UserMessageID: 13, SourceSHA256: sha256.Sum256([]byte("turn")),
	}}
	reconciler := NewDocumentIndexReconciler(
		newMemoryIngestionRepository(), NewDocumentVersionEnqueuer(queue), 10, 4,
		WithConversationIndexRepair(repository, NewConversationTurnEnqueuer(queue)),
	)
	for range 3 {
		if _, err := reconciler.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(repository.afterRunIDs) != 3 || repository.afterRunIDs[0] != 0 || repository.afterRunIDs[1] != 42 || repository.afterRunIDs[2] != 0 {
		t.Fatalf("after run IDs=%v, want [0 42 0]", repository.afterRunIDs)
	}
	if len(queue.tasks) != 2 || queue.tasks[0].Type != TaskContextConversationIndexV1 || queue.tasks[1].Type != TaskContextConversationIndexV1 {
		t.Fatalf("tasks=%+v", queue.tasks)
	}
}

func TestReconcileStableBatchRequeuesQueuedAndExpiredProcessing(t *testing.T) {
	now := time.Now().UTC()
	repository := newMemoryIngestionRepository(memoryVersion(3, DocumentVersionQueued), memoryVersion(1, DocumentVersionProcessing), memoryVersion(2, DocumentVersionReady))
	repository.expireLease(1, now.Add(-time.Minute), 1)
	queue := &recordingTaskEnqueuer{}
	reconciler := NewDocumentIndexReconciler(repository, NewDocumentVersionEnqueuer(queue), 10, 4)
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
	repository.expireLease(1, now.Add(-time.Minute), 4)
	queue := &recordingTaskEnqueuer{}
	reconciler := NewDocumentIndexReconciler(repository, NewDocumentVersionEnqueuer(queue), 10, 4)
	if err := reconciler.Reconcile(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(queue.tasks) != 0 || repository.state(1) != DocumentVersionFailed {
		t.Fatalf("tasks=%d state=%s", len(queue.tasks), repository.state(1))
	}
}

type consistencyRepositoryStub struct {
	profiles  []ContextProfile
	documents map[uint64][]RebuildDocument
	changes   []ProfileIndexCAS
}

func (repository *consistencyRepositoryStub) ListIndexConsistencyProfiles(context.Context, int) ([]ContextProfile, error) {
	return append([]ContextProfile(nil), repository.profiles...), nil
}

func (repository *consistencyRepositoryStub) CompareAndSwapRebuildIndex(_ context.Context, change ProfileIndexCAS) (bool, error) {
	repository.changes = append(repository.changes, change)
	return true, nil
}

func (repository *consistencyRepositoryStub) LoadRebuildDocuments(_ context.Context, profileID uint64) ([]RebuildDocument, error) {
	return repository.documents[profileID], nil
}

type consistencyIndexStub struct {
	aliases                  map[string]string
	collectionErrors         map[string]error
	pointErrors              map[string]error
	switches                 []string
	deletedCollections       []string
	deletedVersionPoint      []uint64
	deletedConversationPoint []uint64
}

func (index *consistencyIndexStub) EnsureCollection(context.Context, contextindex.CollectionSpec) error {
	return nil
}
func (index *consistencyIndexStub) VerifyCollection(_ context.Context, collection string, _ contextindex.ActiveCollection) error {
	return index.collectionErrors[collection]
}
func (index *consistencyIndexStub) SwitchAlias(_ context.Context, alias, collection string) error {
	if index.aliases == nil {
		index.aliases = make(map[string]string)
	}
	index.aliases[alias] = collection
	index.switches = append(index.switches, collection)
	return nil
}
func (index *consistencyIndexStub) AliasTarget(_ context.Context, alias string) (string, bool, error) {
	target, exists := index.aliases[alias]
	return target, exists, nil
}
func (index *consistencyIndexStub) DeleteCollection(_ context.Context, collection string) error {
	index.deletedCollections = append(index.deletedCollections, collection)
	return nil
}
func (index *consistencyIndexStub) DeleteDocumentVersionPoints(_ context.Context, _ string, _, _, versionID uint64) error {
	index.deletedVersionPoint = append(index.deletedVersionPoint, versionID)
	return nil
}
func (index *consistencyIndexStub) DeleteConversationTurnPoint(_ context.Context, _ string, _, _, userMessageID uint64, _ [32]byte) error {
	index.deletedConversationPoint = append(index.deletedConversationPoint, userMessageID)
	return nil
}
func (index *consistencyIndexStub) Upsert(context.Context, string, []contextindex.IndexedPoint) error {
	return nil
}
func (index *consistencyIndexStub) VerifyPoints(_ context.Context, collection string, _ []contextindex.PointRef, _ uint32) error {
	return index.pointErrors[collection]
}

func TestConsistencyRepairsAliasSwitchedBeforeMySQLGeneration(t *testing.T) {
	one, two := uint64(1), uint64(2)
	profile := consistencyProfile(ProfileIndexRebuilding, &one, &two)
	repository := &consistencyRepositoryStub{profiles: []ContextProfile{profile}}
	index := &consistencyIndexStub{aliases: map[string]string{"ctx_profile_7": "ctx_profile_7_g2"}}
	reconciler := consistencyReconciler(repository, index)
	if _, err := reconciler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(repository.changes) != 1 || repository.changes[0].Next.State != ProfileIndexReady ||
		repository.changes[0].Next.ActiveGeneration == nil || *repository.changes[0].Next.ActiveGeneration != two {
		t.Fatalf("changes=%+v", repository.changes)
	}
}

func TestConsistencyRestoresAliasToMySQLActiveGeneration(t *testing.T) {
	two := uint64(2)
	profile := consistencyProfile(ProfileIndexReady, &two, nil)
	repository := &consistencyRepositoryStub{profiles: []ContextProfile{profile}}
	index := &consistencyIndexStub{aliases: map[string]string{"ctx_profile_7": "ctx_profile_7_g1"}}
	reconciler := consistencyReconciler(repository, index)
	if _, err := reconciler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(index.switches) != 1 || index.switches[0] != "ctx_profile_7_g2" || len(repository.changes) != 0 {
		t.Fatalf("switches=%v changes=%+v", index.switches, repository.changes)
	}
}

func TestConsistencyMissingDocumentPointFailsProfile(t *testing.T) {
	two := uint64(2)
	profile := consistencyProfile(ProfileIndexReady, &two, nil)
	chunk := PersistedChunk{ID: 11, Version: 9, Chunk: Chunk{ChunkFactsSHA256: [32]byte{1}}}
	repository := &consistencyRepositoryStub{profiles: []ContextProfile{profile}, documents: map[uint64][]RebuildDocument{
		profile.ID: {{Work: DocumentIndexWork{Profile: profile}, Chunks: []PersistedChunk{chunk}}},
	}}
	index := &consistencyIndexStub{aliases: map[string]string{"ctx_profile_7": "ctx_profile_7_g2"},
		pointErrors: map[string]error{"ctx_profile_7_g2": errors.New("point missing")}}
	reconciler := consistencyReconciler(repository, index)
	if _, err := reconciler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(repository.changes) != 1 || repository.changes[0].Next.State != ProfileIndexFailed ||
		repository.changes[0].Next.ErrorCode == nil || *repository.changes[0].Next.ErrorCode != ErrCodeIndexInconsistent {
		t.Fatalf("changes=%+v", repository.changes)
	}
}

func TestConsistencyMissingActiveCollectionFailsProfile(t *testing.T) {
	two := uint64(2)
	profile := consistencyProfile(ProfileIndexReady, &two, nil)
	repository := &consistencyRepositoryStub{profiles: []ContextProfile{profile}}
	index := &consistencyIndexStub{aliases: map[string]string{"ctx_profile_7": "ctx_profile_7_g2"},
		collectionErrors: map[string]error{"ctx_profile_7_g2": errors.New("collection missing")}}
	if _, err := consistencyReconciler(repository, index).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(repository.changes) != 1 || repository.changes[0].Next.State != ProfileIndexFailed ||
		repository.changes[0].Next.ErrorCode == nil || *repository.changes[0].Next.ErrorCode != ErrCodeIndexInconsistent {
		t.Fatalf("changes=%+v", repository.changes)
	}
}

func consistencyProfile(state ProfileIndexState, active, target *uint64) ContextProfile {
	return ContextProfile{ID: 7, Status: ProfileEnabled, IndexState: state, ActiveIndexGeneration: active,
		TargetIndexGeneration: target, EmbeddingDimensions: 3, DenseDistance: string(contextindex.DistanceCosine)}
}

func consistencyReconciler(repository *consistencyRepositoryStub, index *consistencyIndexStub) *DocumentIndexReconciler {
	ingestion := newMemoryIngestionRepository()
	return NewDocumentIndexReconciler(ingestion, NewDocumentVersionEnqueuer(&recordingTaskEnqueuer{}), 10, 4,
		WithProfileIndexConsistency(repository, index, "ctx"))
}
