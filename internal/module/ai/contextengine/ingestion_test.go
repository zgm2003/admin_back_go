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
func (*memoryIngestionRepository) RecordVersionFailure(context.Context, uint64, uint64, string, string, string, time.Time) error {
	return nil
}
func (repository *memoryIngestionRepository) FinalizeVersion(_ context.Context, attempt DocumentIndexAttempt, code string, requireExpired bool, now time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	version := repository.versions[attempt.VersionID]
	if version == nil || version.state != DocumentVersionProcessing || version.attempts != attempt.AttemptCount || version.token != attempt.LeaseToken || version.expires.After(now) == requireExpired {
		return ErrVersionLeaseLost
	}
	version.state = DocumentVersionFailed
	return nil
}
func (repository *memoryIngestionRepository) ListReconcileCandidates(_ context.Context, now time.Time, limit int) ([]ReconcileCandidate, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	var result []ReconcileCandidate
	for id := uint64(1); len(result) < limit && id < 100; id++ {
		if version := repository.versions[id]; version != nil && (version.state == DocumentVersionQueued || version.state == DocumentVersionProcessing && !version.expires.After(now)) {
			result = append(result, ReconcileCandidate{VersionID: id, State: version.state, AttemptCount: version.attempts, LeaseToken: version.token})
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
	repository.versions[id].token = uint64(attempts)
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

type recordingIngestionRepository struct {
	*memoryIngestionRepository
	stage   string
	code    string
	message string
}

func (repository *recordingIngestionRepository) RecordVersionFailure(_ context.Context, _ uint64, _ uint64, stage, code, message string, _ time.Time) error {
	repository.stage, repository.code, repository.message = stage, code, message
	return nil
}

func TestRetryableIngestionFailureRecordsRootCause(t *testing.T) {
	repository := &recordingIngestionRepository{memoryIngestionRepository: newMemoryIngestionRepository(memoryVersion(7, DocumentVersionProcessing))}
	service := NewDocumentIndexService(DocumentIndexDependencies{Repository: repository})
	work := DocumentIndexWork{Version: ContextDocumentVersion{ID: 7}, Lease: VersionLease{Token: 11}}

	err := service.failAttempt(context.Background(), work, context.DeadlineExceeded)
	classified := classifyIngestionError(err)
	if classified == nil || !classified.Retryable() {
		t.Fatalf("classified=%+v, want retryable error", classified)
	}
	if repository.stage != "index" || repository.code != "ai.context.document_index_failed" || repository.message != "context deadline exceeded" {
		t.Fatalf("recorded stage=%q code=%q message=%q", repository.stage, repository.code, repository.message)
	}
}

func TestSameChunkRowAcceptsMySQLNormalizedLocatorJSON(t *testing.T) {
	stored := contextChunkRow{
		HeadingPath:                   `["admin_back_go Architecture"]`,
		Content:                       "Runtime and Admin contract boundary",
		ContentSHA256:                 []byte{1, 2, 3},
		ChunkFactsSHA256:              []byte{4, 5, 6},
		EmbeddingInputTokenUpperBound: 35,
		LocatorJSON:                   `{"kind": "markdown_block", "schema": "context_locator_v1", "line_end": 3, "line_start": 3, "heading_path": ["admin_back_go Architecture"]}`,
	}
	retry := stored
	retry.LocatorJSON = `{"schema":"context_locator_v1","kind":"markdown_block","line_start":3,"line_end":3,"heading_path":["admin_back_go Architecture"]}`

	if !sameChunkRow(stored, retry) {
		t.Fatal("MySQL JSON normalization changed formatting, not immutable locator facts")
	}
}

func TestDocumentVersionPoliciesRejectMismatchedFrozenVersions(t *testing.T) {
	parser := frozenParser{name: "txt", version: "1"}
	version := ContextDocumentVersion{ParserName: "txt", ParserVersion: "2", ChunkerVersion: ChunkerVersionV1}
	if err := validateDocumentVersionPolicies(version, parser); !errors.Is(err, ErrParserPolicyUnavailable) {
		t.Fatalf("parser policy error=%v", err)
	}

	version.ParserVersion = "1"
	version.ChunkerVersion = "context_chunker_v2"
	if err := validateDocumentVersionPolicies(version, parser); !errors.Is(err, ErrChunkerPolicyUnavailable) {
		t.Fatalf("chunker policy error=%v", err)
	}
}

type frozenParser struct {
	name    string
	version string
}

func (parser frozenParser) Name() string    { return parser.name }
func (parser frozenParser) Version() string { return parser.version }
func (frozenParser) Parse(context.Context, documentparser.Source, documentparser.Limits) ([]documentparser.Block, error) {
	return nil, nil
}
