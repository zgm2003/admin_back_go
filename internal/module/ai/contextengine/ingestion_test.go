package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/infra/documentparser"
)

func TestVersionLeaseRejectsStaleTokenAndDuplicateDelivery(t *testing.T) {
	repository := newMemoryIngestionRepository(memoryVersion(7, DocumentVersionQueued))
	work, disposition, err := repository.AcquireVersionLease(context.Background(), 7, time.Now(), time.Minute)
	lease := work.Lease
	if err != nil || disposition != LeaseAcquired {
		t.Fatalf("lease=%+v disposition=%s err=%v", lease, disposition, err)
	}
	if err := repository.CheckVersionLease(context.Background(), 7, lease.Token+1, time.Now()); !errors.Is(err, ErrVersionLeaseLost) {
		t.Fatalf("stale lease error=%v", err)
	}
	_, disposition, err = repository.AcquireVersionLease(context.Background(), 7, time.Now(), time.Minute)
	if err != nil || disposition != LeaseBusy {
		t.Fatalf("duplicate disposition=%s err=%v", disposition, err)
	}
}

type memoryIngestionRepository struct {
	mu       sync.Mutex
	versions map[uint64]*memoryIngestionVersion
	active   map[uint64]uint64
}

type memoryIngestionVersion struct {
	id         uint64
	documentID uint64
	state      string
	attempts   uint32
	token      uint64
	expires    time.Time
}

func memoryVersion(id uint64, state string) memoryIngestionVersion {
	return memoryIngestionVersion{id: id, documentID: 7, state: state}
}

func newMemoryIngestionRepository(versions ...memoryIngestionVersion) *memoryIngestionRepository {
	repository := &memoryIngestionRepository{versions: map[uint64]*memoryIngestionVersion{}, active: map[uint64]uint64{}}
	for i := range versions {
		copy := versions[i]
		repository.versions[copy.id] = &copy
	}
	return repository
}

func (repository *memoryIngestionRepository) AcquireVersionLease(_ context.Context, id uint64, now time.Time, duration time.Duration) (DocumentIndexWork, LeaseDisposition, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	version := repository.versions[id]
	if version == nil {
		return DocumentIndexWork{}, "", errors.New("not found")
	}
	if version.state == DocumentVersionReady || version.state == DocumentVersionFailed {
		return DocumentIndexWork{}, LeaseTerminal, nil
	}
	if version.state == DocumentVersionProcessing && version.expires.After(now) {
		return DocumentIndexWork{}, LeaseBusy, nil
	}
	version.state, version.attempts, version.token, version.expires = DocumentVersionProcessing, version.attempts+1, uint64(version.attempts+1), now.Add(duration)
	return DocumentIndexWork{Version: ContextDocumentVersion{ID: id}, Lease: VersionLease{Token: version.token, AttemptCount: version.attempts, ExpiresAt: version.expires}}, LeaseAcquired, nil
}

func (repository *memoryIngestionRepository) CheckVersionLease(_ context.Context, id, token uint64, now time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	version := repository.versions[id]
	if version == nil || version.state != DocumentVersionProcessing || version.token != token || !version.expires.After(now) {
		return ErrVersionLeaseLost
	}
	return nil
}
func (*memoryIngestionRepository) StoreSourceSHA256(context.Context, uint64, uint64, [sha256.Size]byte, time.Time) error {
	return nil
}
func (*memoryIngestionRepository) UpsertImmutableChunks(context.Context, uint64, uint64, []Chunk, time.Time) ([]PersistedChunk, error) {
	return nil, nil
}
func (*memoryIngestionRepository) ActivateVersion(context.Context, VersionActivation) error {
	return nil
}
func (repository *memoryIngestionRepository) FailVersion(_ context.Context, id, token uint64, stage, code, message string, now time.Time) error {
	return repository.forceFailed(id, stage, code)
}
func (repository *memoryIngestionRepository) FinalizeVersion(_ context.Context, id uint64, code string, attemptLimit uint32, now time.Time) error {
	return repository.forceFailed(id, "index", code)
}
func (repository *memoryIngestionRepository) ListReconcileCandidates(_ context.Context, now time.Time, limit int) ([]ReconcileCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	var result []ReconcileCandidate
	for id := uint64(1); len(result) < limit && id < 100; id++ {
		if version := repository.versions[id]; version != nil && (version.state == DocumentVersionQueued || version.state == DocumentVersionProcessing && !version.expires.After(now)) {
			result = append(result, ReconcileCandidate{VersionID: id, State: version.state, AttemptCount: version.attempts})
		}
	}
	return result, nil
}
func (repository *memoryIngestionRepository) forceReady(id uint64) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	version := repository.versions[id]
	if version == nil {
		return errors.New("not found")
	}
	version.state = DocumentVersionReady
	if id > repository.active[version.documentID] {
		repository.active[version.documentID] = id
	}
	return nil
}
func (repository *memoryIngestionRepository) forceFailed(id uint64, stage, code string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	version := repository.versions[id]
	if version == nil {
		return errors.New("not found")
	}
	version.state = DocumentVersionFailed
	return nil
}
func (repository *memoryIngestionRepository) activeVersion(documentID uint64) uint64 {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.active[documentID]
}
func (repository *memoryIngestionRepository) expireLease(id uint64, expires time.Time, attempts uint32) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.versions[id].state = DocumentVersionProcessing
	repository.versions[id].expires = expires
	repository.versions[id].attempts = attempts
}
func (repository *memoryIngestionRepository) state(id uint64) string {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.versions[id].state
}

var _ IngestionRepository = (*memoryIngestionRepository)(nil)
var _ = documentparser.ErrMalformedDocument

func TestActivationRejectsOlderReadyVersion(t *testing.T) {
	repository := newMemoryIngestionRepository(memoryVersion(11, DocumentVersionProcessing), memoryVersion(12, DocumentVersionProcessing))
	if err := repository.forceReady(12); err != nil {
		t.Fatal(err)
	}
	if err := repository.forceReady(11); err != nil {
		t.Fatal(err)
	}
	if got := repository.activeVersion(7); got != 12 {
		t.Fatalf("active_version_id=%d, want 12", got)
	}
}

func TestFailedNewestVersionPreservesPreviousActive(t *testing.T) {
	repository := newMemoryIngestionRepository(memoryVersion(11, DocumentVersionProcessing), memoryVersion(12, DocumentVersionProcessing))
	if err := repository.forceReady(11); err != nil {
		t.Fatal(err)
	}
	if err := repository.forceFailed(12, "parse", "ai.context.document_parse_failed"); err != nil {
		t.Fatal(err)
	}
	if got := repository.activeVersion(7); got != 11 {
		t.Fatalf("active_version_id=%d, want 11", got)
	}
}

func TestDocumentIndexClassifiesTransientAndPermanentFailures(t *testing.T) {
	if got := classifyIngestionError(context.DeadlineExceeded); got == nil || !got.Retryable() {
		t.Fatalf("deadline classification=%+v", got)
	}
	if got := classifyIngestionError(ErrChunkFactsConflict); got == nil || got.Retryable() {
		t.Fatalf("chunk conflict classification=%+v", got)
	}
}
