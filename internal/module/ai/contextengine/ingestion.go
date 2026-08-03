package contextengine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/documentparser"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/storage"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DocumentVersionProcessing = "processing"
	DocumentVersionFailed     = "failed"
	DocumentIndexMaxRetry     = 3
	documentIndexLease        = 5 * time.Minute
	documentEmbeddingBatch    = 64
)

var (
	ErrVersionLeaseLost           = errors.New("context document version lease lost")
	ErrVersionLeaseBusy           = errors.New("context document version lease is active")
	ErrDocumentIndexNotConfigured = errors.New("context document index service is not configured")
	ErrEmbeddingDimension         = errors.New("context embedding dimension mismatch")
	ErrIndexGenerationUnavailable = errors.New("context profile active index generation is unavailable")
	ErrParserPolicyUnavailable    = errors.New("context document parser policy is unavailable")
	ErrChunkerPolicyUnavailable   = errors.New("context document chunker policy is unavailable")
	ErrIndexVerificationFailed    = errors.New("context document index verification failed")
	ErrSourceFactsInvalid         = errors.New("context document source facts are invalid")
)

type LeaseDisposition string

const (
	LeaseAcquired LeaseDisposition = "acquired"
	LeaseBusy     LeaseDisposition = "busy"
	LeaseTerminal LeaseDisposition = "terminal"
)

type VersionLease struct {
	Token        uint64
	AttemptCount uint32
	ExpiresAt    time.Time
}

type DocumentIndexAttempt struct {
	VersionID      uint64
	LeaseToken     uint64
	AttemptCount   uint32
	IdempotencyKey string
}

type DocumentIndexWork struct {
	Version         ContextDocumentVersion
	Profile         ContextProfile
	Document        ContextDocument
	Platform        string
	SpaceID         uint64
	ConversationID  uint64
	UserID          uint64
	IndexGeneration uint64
	Lease           VersionLease
}

type PersistedChunk struct {
	ID      uint64
	Version uint64
	Chunk   Chunk
}

type VersionActivation struct {
	VersionID                     uint64
	ProfileID                     uint64
	IndexGeneration               uint64
	LeaseToken                    uint64
	SourceSHA256                  [sha256.Size]byte
	ChunkCount                    uint32
	EmbeddingInputTokenUpperBound uint64
	EmbeddingRequestCount         uint32
	EmbeddingInputTokens          uint64
	FinishedAt                    time.Time
}

type ReconcileCandidate struct {
	VersionID    uint64
	State        string
	AttemptCount uint32
	LeaseToken   uint64
}

type IngestionRepository interface {
	AcquireVersionLease(context.Context, uint64, time.Time, time.Duration) (DocumentIndexWork, LeaseDisposition, error)
	CheckVersionLease(context.Context, uint64, uint64, time.Time) error
	StoreSourceSHA256(context.Context, uint64, uint64, [sha256.Size]byte, time.Time) error
	UpsertImmutableChunks(context.Context, uint64, uint64, []Chunk, time.Time) ([]PersistedChunk, error)
	ActivateVersion(context.Context, VersionActivation) error
	FailVersion(context.Context, uint64, uint64, string, string, string, time.Time) error
	FinalizeVersion(context.Context, DocumentIndexAttempt, string, bool, time.Time) error
	ListReconcileCandidates(context.Context, time.Time, int) ([]ReconcileCandidate, error)
}

type DocumentParser interface {
	Resolve(string, string) (documentparser.Parser, error)
}

type EmbeddingResolver interface {
	ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error)
}

type IndexWriter interface {
	Upsert(context.Context, string, []contextindex.IndexedPoint) error
	VerifyPoints(context.Context, string, []contextindex.PointRef, uint32) error
}

type DocumentIndexDependencies struct {
	Repository       IngestionRepository
	Objects          storage.ConditionalObjectReader
	Parser           DocumentParser
	Embeddings       EmbeddingResolver
	Index            IndexWriter
	CollectionPrefix string
	Limits           documentparser.Limits
	Now              func() time.Time
}

type DocumentIndexService struct{ deps DocumentIndexDependencies }

func NewDocumentIndexService(deps DocumentIndexDependencies) *DocumentIndexService {
	if deps.Parser == nil {
		deps.Parser = documentparser.NewRegistry()
	}
	if deps.Limits.MaxSourceBytes == 0 {
		deps.Limits = documentparser.DefaultLimits()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &DocumentIndexService{deps: deps}
}

func (service *DocumentIndexService) IndexDocument(ctx context.Context, versionID uint64) (DocumentIndexAttempt, error) {
	if service == nil || service.deps.Repository == nil || service.deps.Objects == nil || service.deps.Parser == nil ||
		service.deps.Embeddings == nil || service.deps.Index == nil || strings.TrimSpace(service.deps.CollectionPrefix) == "" || versionID == 0 {
		return DocumentIndexAttempt{}, apperror.Wrap("ai.context.index_not_configured", apperror.CategoryInternal, http.StatusInternalServerError,
			apperror.Permanent, "", nil, "context document index is not configured", ErrDocumentIndexNotConfigured)
	}
	now := service.deps.Now().UTC()
	work, disposition, err := service.deps.Repository.AcquireVersionLease(ctx, versionID, now, documentIndexLease)
	if err != nil {
		return DocumentIndexAttempt{}, classifyIngestionError(err)
	}
	if disposition == LeaseTerminal || disposition == LeaseBusy {
		return DocumentIndexAttempt{}, nil
	}
	attempt := DocumentIndexAttempt{VersionID: versionID, LeaseToken: work.Lease.Token, AttemptCount: work.Lease.AttemptCount}
	facts := DocumentIndexFacts{
		VersionID:      work.Version.ID,
		ProfileID:      work.Profile.ID,
		ParserVersion:  work.Version.ParserVersion,
		ChunkerVersion: work.Version.ChunkerVersion,
	}
	if len(work.Version.SourceFactsSHA256) != sha256.Size {
		return attempt, service.failAttempt(ctx, work, ErrSourceFactsInvalid)
	}
	copy(facts.SourceFactsSHA256[:], work.Version.SourceFactsSHA256)
	attempt.IdempotencyKey, err = DocumentIndexIdempotencyKey(facts)
	if err != nil {
		return attempt, service.failAttempt(ctx, work, err)
	}
	if err := service.runPipeline(ctx, work); err != nil {
		classified := classifyIngestionError(err)
		if classified != nil && !classified.Retryable() {
			if failErr := service.deps.Repository.FailVersion(ctx, versionID, work.Lease.Token, failureStage(err), classified.Code, classified.Message, service.deps.Now().UTC()); failErr != nil {
				return attempt, classifyIngestionError(failErr)
			}
		}
		return attempt, classified
	}
	return attempt, nil
}

func (service *DocumentIndexService) FinalizeDocumentIndex(ctx context.Context, attempt DocumentIndexAttempt, code string, deliveryLimit int) error {
	if service == nil || service.deps.Repository == nil {
		return ErrDocumentIndexNotConfigured
	}
	if attempt.VersionID == 0 || attempt.LeaseToken == 0 || attempt.AttemptCount == 0 || deliveryLimit != int(attempt.AttemptCount) {
		return nil
	}
	if strings.TrimSpace(code) == "" {
		code = "ai.context.index_retry_exhausted"
	}
	err := service.deps.Repository.FinalizeVersion(ctx, attempt, code, false, service.deps.Now().UTC())
	if errors.Is(err, ErrVersionLeaseLost) {
		return nil
	}
	return err
}

func (service *DocumentIndexService) failAttempt(ctx context.Context, work DocumentIndexWork, cause error) error {
	classified := classifyIngestionError(cause)
	if classified == nil || classified.Retryable() {
		return classified
	}
	if err := service.deps.Repository.FailVersion(ctx, work.Version.ID, work.Lease.Token, failureStage(cause), classified.Code, classified.Message, service.deps.Now().UTC()); err != nil {
		return classifyIngestionError(err)
	}
	return classified
}

func (service *DocumentIndexService) runPipeline(ctx context.Context, work DocumentIndexWork) error {
	check := func() error {
		return service.deps.Repository.CheckVersionLease(ctx, work.Version.ID, work.Lease.Token, service.deps.Now().UTC())
	}
	if work.IndexGeneration == 0 {
		return ErrIndexGenerationUnavailable
	}
	if err := check(); err != nil {
		return err
	}
	input := storage.ConditionalObjectInput{StorageProvider: work.Version.SourceStorageProvider, ObjectKey: work.Version.SourceObjectKey,
		ETag: work.Version.SourceETag, Size: work.Version.SourceSize}
	body, metadata, err := service.deps.Objects.Open(ctx, input)
	if err != nil {
		return err
	}
	defer body.Close()
	if metadata.ETag != work.Version.SourceETag || metadata.Size != work.Version.SourceSize {
		return storage.ErrConditionalObjectVersionChanged
	}
	hasher := sha256.New()
	parser, err := service.deps.Parser.Resolve(work.Version.SourceFilename, work.Version.SourceMIMEType)
	if err != nil {
		return err
	}
	if err := validateDocumentVersionPolicies(work.Version, parser); err != nil {
		return err
	}
	blocks, err := parser.Parse(ctx, documentparser.Source{Filename: work.Version.SourceFilename,
		MIMEType: work.Version.SourceMIMEType, Size: work.Version.SourceSize, Reader: io.TeeReader(body, hasher)}, service.deps.Limits)
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	var sourceSHA [sha256.Size]byte
	copy(sourceSHA[:], hasher.Sum(nil))
	if err := service.deps.Repository.StoreSourceSHA256(ctx, work.Version.ID, work.Lease.Token, sourceSHA, service.deps.Now().UTC()); err != nil {
		return err
	}
	counter, err := infraai.ResolveTokenCounter(work.Profile.EmbeddingTokenCounterID)
	if err != nil {
		return err
	}
	chunker, err := NewChunker(counter, work.Profile.EmbeddingMaxInputTokens)
	if err != nil {
		return err
	}
	structural := make([]StructuralBlock, len(blocks))
	for i, block := range blocks {
		structural[i] = StructuralBlock{Ordinal: block.Ordinal, Text: block.Text, HeadingPath: block.HeadingPath, Locator: locatorFromParser(block.Locator)}
	}
	chunks, err := chunker.Chunk(structural)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return documentparser.ErrMalformedDocument
	}
	if err := check(); err != nil {
		return err
	}
	persisted, err := service.deps.Repository.UpsertImmutableChunks(ctx, work.Version.ID, work.Lease.Token, chunks, service.deps.Now().UTC())
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	client, err := service.deps.Embeddings.ResolveEmbedding(ctx, work.Profile)
	if err != nil {
		return err
	}
	collection := collectionName(work, service.deps.CollectionPrefix)
	expectedRefs := make([]contextindex.PointRef, 0, len(persisted))
	var requests uint32
	var inputTokens uint64
	for start := 0; start < len(persisted); start += documentEmbeddingBatch {
		end := min(start+documentEmbeddingBatch, len(persisted))
		texts := make([]string, end-start)
		for i := start; i < end; i++ {
			texts[i-start] = persisted[i].Chunk.IndexText
		}
		result, embedErr := client.Embed(ctx, texts)
		if embedErr != nil {
			return embedErr
		}
		requests++
		if result.Usage.PromptTokens > 0 {
			inputTokens += uint64(result.Usage.PromptTokens)
		}
		if len(result.Vectors) != len(texts) {
			return ErrEmbeddingDimension
		}
		batchPoints := make([]contextindex.IndexedPoint, 0, len(result.Vectors))
		for i, vector := range result.Vectors {
			if len(vector) != int(work.Profile.EmbeddingDimensions) {
				return ErrEmbeddingDimension
			}
			point, pointErr := documentChunkPoint(work, persisted[start+i], vector)
			if pointErr != nil {
				return pointErr
			}
			batchPoints = append(batchPoints, point)
			expectedRefs = append(expectedRefs, point.Metadata.Ref)
		}
		if err := service.deps.Index.Upsert(ctx, collection, batchPoints); err != nil {
			return err
		}
		if err := check(); err != nil {
			return err
		}
	}
	if len(expectedRefs) != len(chunks) {
		return errors.New("document point count differs from persisted chunk count")
	}
	for start := 0; start < len(expectedRefs); start += documentEmbeddingBatch {
		end := min(start+documentEmbeddingBatch, len(expectedRefs))
		if err := service.deps.Index.VerifyPoints(ctx, collection, expectedRefs[start:end], work.Profile.EmbeddingDimensions); err != nil {
			return fmt.Errorf("%w: %v", ErrIndexVerificationFailed, err)
		}
	}
	if err := check(); err != nil {
		return err
	}
	var bound uint64
	for _, chunk := range chunks {
		bound += uint64(chunk.EmbeddingInputTokenUpperBound)
	}
	return service.deps.Repository.ActivateVersion(ctx, VersionActivation{VersionID: work.Version.ID, ProfileID: work.Profile.ID,
		IndexGeneration: work.IndexGeneration, LeaseToken: work.Lease.Token,
		SourceSHA256: sourceSHA, ChunkCount: uint32(len(chunks)), EmbeddingInputTokenUpperBound: bound,
		EmbeddingRequestCount: requests, EmbeddingInputTokens: inputTokens, FinishedAt: service.deps.Now().UTC()})
}

func collectionName(work DocumentIndexWork, prefix string) string {
	return fmt.Sprintf("%s_profile_%d_g%d", strings.TrimSpace(prefix), work.Profile.ID, work.IndexGeneration)
}

func validateDocumentVersionPolicies(version ContextDocumentVersion, parser documentparser.Parser) error {
	if parser == nil || parser.Name() != strings.TrimSpace(version.ParserName) || parser.Version() != strings.TrimSpace(version.ParserVersion) {
		return ErrParserPolicyUnavailable
	}
	if strings.TrimSpace(version.ChunkerVersion) != ChunkerVersionV1 {
		return ErrChunkerPolicyUnavailable
	}
	return nil
}

func documentChunkPoint(work DocumentIndexWork, persisted PersistedChunk, dense []float32) (contextindex.IndexedPoint, error) {
	pointID, err := PointID(work.Profile.ID, contextindex.SourceKindDocumentChunk, persisted.ID, persisted.Chunk.ChunkFactsSHA256)
	if err != nil {
		return contextindex.IndexedPoint{}, err
	}
	ref, err := contextindex.NewPointRef(pointID, work.Profile.ID, work.IndexGeneration, contextindex.SourceKindDocumentChunk, persisted.ID, persisted.Chunk.ChunkFactsSHA256)
	if err != nil {
		return contextindex.IndexedPoint{}, err
	}
	metadata := contextindex.PointMetadata{Ref: ref, Platform: work.Platform, DocumentID: work.Document.ID,
		DocumentVersionID: work.Version.ID, ChunkID: persisted.ID}
	if work.SpaceID != 0 {
		metadata.ScopeKind, metadata.SpaceID = contextindex.ScopeKindSpace, work.SpaceID
	} else {
		metadata.ScopeKind, metadata.ConversationID, metadata.UserID = contextindex.ScopeKindConversation, work.ConversationID, work.UserID
	}
	sparse, err := EncodeSparse(persisted.Chunk.IndexText)
	if err != nil {
		return contextindex.IndexedPoint{}, err
	}
	return contextindex.NewIndexedPoint(metadata, dense, sparse)
}

func locatorFromParser(locator documentparser.ContextLocatorV1) ContextLocatorV1 {
	return ContextLocatorV1{Schema: locator.Schema, Kind: locator.Kind, Page: locator.Page, Paragraph: locator.Paragraph,
		LineStart: locator.LineStart, LineEnd: locator.LineEnd, RowStart: locator.RowStart, RowEnd: locator.RowEnd,
		Sheet: locator.Sheet, CellStart: locator.CellStart, CellEnd: locator.CellEnd, HeadingPath: append([]string(nil), locator.HeadingPath...)}
}

func classifyIngestionError(err error) *apperror.Error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	permanent := errors.Is(err, documentparser.ErrDocumentParseFailed) || errors.Is(err, documentparser.ErrMalformedDocument) ||
		errors.Is(err, storage.ErrConditionalObjectVersionChanged) || errors.Is(err, ErrChunkFactsConflict) ||
		errors.Is(err, ErrEmbeddingDimension) || errors.Is(err, ErrIndexGenerationUnavailable) ||
		errors.Is(err, ErrParserPolicyUnavailable) || errors.Is(err, ErrChunkerPolicyUnavailable) || errors.Is(err, ErrSourceFactsInvalid)
	if permanent {
		return apperror.Wrap("ai.context.document_index_invalid", apperror.CategoryValidation, http.StatusUnprocessableEntity,
			apperror.Permanent, "", nil, "context document cannot be indexed", err)
	}
	return apperror.Wrap("ai.context.document_index_failed", apperror.CategoryDependency, http.StatusServiceUnavailable,
		apperror.Retryable, "", nil, "context document indexing failed", err)
}

func failureStage(err error) string {
	switch {
	case errors.Is(err, documentparser.ErrDocumentParseFailed), errors.Is(err, documentparser.ErrMalformedDocument):
		return "parse"
	case errors.Is(err, ErrChunkFactsConflict):
		return "chunk"
	case errors.Is(err, ErrEmbeddingDimension), errors.Is(err, infraai.ErrEmbeddingFailed):
		return "embedding"
	case errors.Is(err, storage.ErrConditionalObjectVersionChanged):
		return "source"
	default:
		return "index"
	}
}

type ingestionVersionRow struct {
	ContextDocumentVersion
	SourceSHA256   []byte     `gorm:"column:source_sha256"`
	AttemptCount   uint32     `gorm:"column:attempt_count"`
	LeaseToken     *uint64    `gorm:"column:lease_token"`
	LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
	StartedAt      *time.Time `gorm:"column:started_at"`
}

func (ingestionVersionRow) TableName() string { return "ai_context_document_versions" }

type contextChunkRow struct {
	ID                            uint64 `gorm:"column:id;primaryKey"`
	DocumentVersionID             uint64 `gorm:"column:document_version_id"`
	Ordinal                       uint32 `gorm:"column:ordinal"`
	HeadingPath                   string `gorm:"column:heading_path"`
	Content                       string `gorm:"column:content"`
	ContentSHA256                 []byte `gorm:"column:content_sha256"`
	ChunkFactsSHA256              []byte `gorm:"column:chunk_facts_sha256"`
	EmbeddingInputTokenUpperBound uint64 `gorm:"column:embedding_input_token_upper_bound"`
	LocatorJSON                   string `gorm:"column:locator_json"`
}

func (contextChunkRow) TableName() string { return "ai_context_chunks" }

type GormIngestionRepository struct{ db *gorm.DB }

func NewIngestionRepository(client *database.Client) *GormIngestionRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormIngestionRepository{db: client.Gorm}
}

func (repository *GormIngestionRepository) AcquireVersionLease(ctx context.Context, versionID uint64, now time.Time, duration time.Duration) (DocumentIndexWork, LeaseDisposition, error) {
	if repository == nil || repository.db == nil || versionID == 0 || duration <= 0 {
		return DocumentIndexWork{}, "", ErrDocumentIndexNotConfigured
	}
	var lease VersionLease
	disposition := LeaseAcquired
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row ingestionVersionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", versionID).Take(&row).Error; err != nil {
			return err
		}
		if row.State == DocumentVersionReady || row.State == DocumentVersionFailed {
			disposition = LeaseTerminal
			return nil
		}
		if row.State == DocumentVersionProcessing && row.LeaseExpiresAt != nil && row.LeaseExpiresAt.After(now) {
			disposition = LeaseBusy
			return nil
		}
		token, err := newLeaseToken()
		if err != nil {
			return err
		}
		expires := now.Add(duration)
		started := now
		if row.StartedAt != nil {
			started = *row.StartedAt
		}
		result := tx.Model(&ingestionVersionRow{}).Where("id = ? AND state IN ?", versionID, []string{DocumentVersionQueued, DocumentVersionProcessing}).
			Updates(map[string]any{"state": DocumentVersionProcessing, "attempt_count": gorm.Expr("attempt_count + 1"),
				"lease_token": token, "lease_expires_at": expires, "started_at": started})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionLeaseLost
		}
		lease = VersionLease{Token: token, AttemptCount: row.AttemptCount + 1, ExpiresAt: expires}
		return nil
	})
	if err != nil || disposition != LeaseAcquired {
		return DocumentIndexWork{}, disposition, err
	}
	work, err := repository.loadWork(ctx, versionID)
	if err != nil {
		return DocumentIndexWork{}, "", err
	}
	work.Lease = lease
	return work, disposition, nil
}

func (repository *GormIngestionRepository) LoadDocumentIndexFacts(ctx context.Context, versionID uint64) (DocumentIndexFacts, error) {
	if repository == nil || repository.db == nil || versionID == 0 {
		return DocumentIndexFacts{}, ErrDocumentIndexNotConfigured
	}
	var row struct {
		VersionID       uint64 `gorm:"column:id"`
		ProfileID       uint64 `gorm:"column:profile_id"`
		SourceFactsHash []byte `gorm:"column:source_facts_sha256"`
		ParserVersion   string `gorm:"column:parser_version"`
		ChunkerVersion  string `gorm:"column:chunker_version"`
	}
	if err := repository.db.WithContext(ctx).Table("ai_context_document_versions").Select("id, profile_id, source_facts_sha256, parser_version, chunker_version").Where("id = ?", versionID).Take(&row).Error; err != nil {
		return DocumentIndexFacts{}, err
	}
	facts := DocumentIndexFacts{VersionID: row.VersionID, ProfileID: row.ProfileID, ParserVersion: row.ParserVersion, ChunkerVersion: row.ChunkerVersion}
	if len(row.SourceFactsHash) != sha256.Size {
		return DocumentIndexFacts{}, ErrSourceFactsInvalid
	}
	copy(facts.SourceFactsSHA256[:], row.SourceFactsHash)
	return facts, nil
}

func (repository *GormIngestionRepository) loadWork(ctx context.Context, versionID uint64) (DocumentIndexWork, error) {
	var row ingestionVersionRow
	if err := repository.db.WithContext(ctx).Where("id = ?", versionID).Take(&row).Error; err != nil {
		return DocumentIndexWork{}, err
	}
	var profile ContextProfile
	if err := repository.db.WithContext(ctx).Where("id = ?", row.ProfileID).Take(&profile).Error; err != nil {
		return DocumentIndexWork{}, err
	}
	if profile.ActiveIndexGeneration == nil || *profile.ActiveIndexGeneration == 0 {
		return DocumentIndexWork{}, ErrIndexGenerationUnavailable
	}
	var document ContextDocument
	if err := repository.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", row.DocumentID).Take(&document).Error; err != nil {
		return DocumentIndexWork{}, err
	}
	work := DocumentIndexWork{Version: row.ContextDocumentVersion, Profile: profile, Document: document, IndexGeneration: *profile.ActiveIndexGeneration}
	if document.SpaceID != nil {
		var space ContextSpace
		if err := repository.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", *document.SpaceID).Take(&space).Error; err != nil {
			return DocumentIndexWork{}, err
		}
		if space.ProfileID != profile.ID {
			return DocumentIndexWork{}, errors.New("document space profile changed")
		}
		work.Platform, work.SpaceID = space.Platform, space.ID
	} else if document.ConversationID != nil {
		var conversation struct {
			Platform string `gorm:"column:platform"`
			UserID   uint64 `gorm:"column:user_id"`
		}
		if err := repository.db.WithContext(ctx).Table("ai_conversations").Select("platform, user_id").Where("id = ?", *document.ConversationID).Take(&conversation).Error; err != nil {
			return DocumentIndexWork{}, err
		}
		work.Platform, work.ConversationID, work.UserID = conversation.Platform, *document.ConversationID, conversation.UserID
	}
	return work, nil
}

func (repository *GormIngestionRepository) CheckVersionLease(ctx context.Context, versionID, token uint64, now time.Time) error {
	var count int64
	err := repository.db.WithContext(ctx).Model(&ingestionVersionRow{}).Where("id = ? AND state = ? AND lease_token = ? AND lease_expires_at > ?", versionID, DocumentVersionProcessing, token, now).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrVersionLeaseLost
	}
	return nil
}

func (repository *GormIngestionRepository) StoreSourceSHA256(ctx context.Context, versionID, token uint64, digest [sha256.Size]byte, now time.Time) error {
	result := repository.db.WithContext(ctx).Model(&ingestionVersionRow{}).
		Where("id = ? AND state = ? AND lease_token = ? AND lease_expires_at > ? AND (source_sha256 IS NULL OR source_sha256 = ?)", versionID, DocumentVersionProcessing, token, now, digest[:]).
		Update("source_sha256", digest[:])
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVersionLeaseLost
	}
	return nil
}

func (repository *GormIngestionRepository) UpsertImmutableChunks(ctx context.Context, versionID, token uint64, chunks []Chunk, now time.Time) ([]PersistedChunk, error) {
	if err := repository.CheckVersionLease(ctx, versionID, token, now); err != nil {
		return nil, err
	}
	result := make([]PersistedChunk, 0, len(chunks))
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, chunk := range chunks {
			heading, _ := json.Marshal(chunk.HeadingPath)
			locator, _ := json.Marshal(chunk.Locator)
			row := contextChunkRow{DocumentVersionID: versionID, Ordinal: chunk.Ordinal, HeadingPath: string(heading), Content: chunk.Text,
				ContentSHA256: chunk.ContentSHA256[:], ChunkFactsSHA256: chunk.ChunkFactsSHA256[:],
				EmbeddingInputTokenUpperBound: uint64(chunk.EmbeddingInputTokenUpperBound), LocatorJSON: string(locator)}
			var existing contextChunkRow
			err := tx.Where("document_version_id = ? AND ordinal = ?", versionID, chunk.Ordinal).Take(&existing).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				existing = row
			case err != nil:
				return err
			case !sameChunkRow(existing, row):
				return ErrChunkFactsConflict
			}
			result = append(result, PersistedChunk{ID: existing.ID, Version: versionID, Chunk: chunk})
		}
		return nil
	})
	return result, err
}

func sameChunkRow(left, right contextChunkRow) bool {
	return left.HeadingPath == right.HeadingPath && left.Content == right.Content && string(left.ContentSHA256) == string(right.ContentSHA256) &&
		string(left.ChunkFactsSHA256) == string(right.ChunkFactsSHA256) && left.EmbeddingInputTokenUpperBound == right.EmbeddingInputTokenUpperBound && left.LocatorJSON == right.LocatorJSON
}

func (repository *GormIngestionRepository) ActivateVersion(ctx context.Context, input VersionActivation) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var version ingestionVersionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", input.VersionID).Take(&version).Error; err != nil {
			return err
		}
		if version.State == DocumentVersionReady {
			return nil
		}
		if version.State != DocumentVersionProcessing || version.LeaseToken == nil || *version.LeaseToken != input.LeaseToken || version.LeaseExpiresAt == nil || !version.LeaseExpiresAt.After(input.FinishedAt) {
			return ErrVersionLeaseLost
		}
		if version.ProfileID != input.ProfileID {
			return ErrVersionLeaseLost
		}
		var profile ContextProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", version.ProfileID).Take(&profile).Error; err != nil {
			return err
		}
		if profile.ActiveIndexGeneration == nil || *profile.ActiveIndexGeneration != input.IndexGeneration {
			return ErrIndexGenerationUnavailable
		}
		var document ContextDocument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", version.DocumentID).Take(&document).Error; err != nil {
			return err
		}
		if document.SpaceID != nil {
			var space ContextSpace
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", *document.SpaceID).Take(&space).Error; err != nil {
				return err
			}
			if space.ProfileID != version.ProfileID {
				return ErrVersionLeaseLost
			}
		} else if document.ConversationID != nil {
			var authority struct {
				ContextProfileID *uint64 `gorm:"column:context_profile_id"`
			}
			if err := tx.Table("ai_conversations AS c").Select("a.context_profile_id").
				Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = 2").
				Where("c.id = ? AND c.is_del = 2", *document.ConversationID).Take(&authority).Error; err != nil {
				return err
			}
			if authority.ContextProfileID == nil || *authority.ContextProfileID != version.ProfileID {
				return ErrVersionLeaseLost
			}
		} else {
			return ErrVersionLeaseLost
		}
		updates := map[string]any{"state": DocumentVersionReady, "source_sha256": input.SourceSHA256[:], "chunk_count": input.ChunkCount,
			"embedding_input_token_upper_bound": input.EmbeddingInputTokenUpperBound, "embedding_request_count": input.EmbeddingRequestCount,
			"embedding_input_tokens": input.EmbeddingInputTokens, "finished_at": input.FinishedAt, "lease_token": nil, "lease_expires_at": nil}
		if err := tx.Model(&ingestionVersionRow{}).Where("id = ?", input.VersionID).Updates(updates).Error; err != nil {
			return err
		}
		var newest uint64
		if err := tx.Model(&ingestionVersionRow{}).Where("document_id = ? AND state = ?", version.DocumentID, DocumentVersionReady).Select("MAX(id)").Scan(&newest).Error; err != nil {
			return err
		}
		return tx.Model(&ContextDocument{}).Where("id = ?", version.DocumentID).Update("active_version_id", newest).Error
	})
}

func (repository *GormIngestionRepository) FailVersion(ctx context.Context, versionID, token uint64, stage, code, message string, now time.Time) error {
	result := repository.db.WithContext(ctx).Model(&ingestionVersionRow{}).
		Where("id = ? AND state = ? AND lease_token = ? AND lease_expires_at > ?", versionID, DocumentVersionProcessing, token, now).
		Updates(map[string]any{"state": DocumentVersionFailed, "failure_stage": stage, "error_code": code, "error_message": sanitizeFailure(message),
			"finished_at": now, "lease_token": nil, "lease_expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVersionLeaseLost
	}
	return nil
}

func (repository *GormIngestionRepository) FinalizeVersion(ctx context.Context, attempt DocumentIndexAttempt, code string, requireExpiredLease bool, now time.Time) error {
	query := repository.db.WithContext(ctx).Model(&ingestionVersionRow{}).
		Where("id = ? AND state = ? AND attempt_count = ? AND lease_token = ?", attempt.VersionID, DocumentVersionProcessing, attempt.AttemptCount, attempt.LeaseToken)
	if requireExpiredLease {
		query = query.Where("lease_expires_at <= ?", now)
	} else {
		query = query.Where("lease_expires_at > ?", now)
	}
	result := query.
		Updates(map[string]any{"state": DocumentVersionFailed, "failure_stage": "index", "error_code": code,
			"error_message": "context document indexing retry budget exhausted", "finished_at": now, "lease_token": nil, "lease_expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVersionLeaseLost
	}
	return nil
}

func (repository *GormIngestionRepository) ListReconcileCandidates(ctx context.Context, now time.Time, limit int) ([]ReconcileCandidate, error) {
	if limit <= 0 {
		return nil, errors.New("reconcile batch limit must be positive")
	}
	var rows []ReconcileCandidate
	err := repository.db.WithContext(ctx).Table("ai_context_document_versions").Select("id AS version_id, state, attempt_count, COALESCE(lease_token, 0) AS lease_token").
		Where("state = ? OR (state = ? AND lease_expires_at <= ?)", DocumentVersionQueued, DocumentVersionProcessing, now).
		Order("id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func newLeaseToken() (uint64, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return 0, err
	}
	token := binary.BigEndian.Uint64(data[:])
	if token == 0 {
		token = 1
	}
	return token, nil
}

func sanitizeFailure(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "context document indexing failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

type GormEmbeddingResolver struct {
	db      *gorm.DB
	factory infraai.EmbeddingFactory
	box     secretbox.Box
}

func NewEmbeddingResolver(client *database.Client, factory infraai.EmbeddingFactory, box secretbox.Box) *GormEmbeddingResolver {
	if client == nil {
		return nil
	}
	return &GormEmbeddingResolver{db: client.Gorm, factory: factory, box: box}
}

func (resolver *GormEmbeddingResolver) ResolveEmbedding(ctx context.Context, profile ContextProfile) (infraai.EmbeddingClient, error) {
	if resolver == nil || resolver.db == nil || resolver.factory == nil {
		return nil, ErrDocumentIndexNotConfigured
	}
	var row struct {
		ModelID         string `gorm:"column:model_id"`
		ModelKind       string `gorm:"column:model_kind"`
		ModelStatus     int    `gorm:"column:model_status"`
		EngineType      string `gorm:"column:engine_type"`
		BaseURL         string `gorm:"column:base_url"`
		APIKeyEnc       string `gorm:"column:api_key_enc"`
		ProviderStatus  int    `gorm:"column:provider_status"`
		ProviderDeleted int    `gorm:"column:provider_deleted"`
	}
	err := resolver.db.WithContext(ctx).Table("ai_provider_models AS pm").Select("pm.model_id, pm.model_kind, pm.status AS model_status, p.engine_type, p.base_url, p.api_key_enc, p.status AS provider_status, p.is_del AS provider_deleted").
		Joins("JOIN ai_providers AS p ON p.id = pm.provider_id").Where("pm.id = ?", profile.EmbeddingProviderModelID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ModelKind != string(aiprovider.ModelKindEmbedding) || row.ModelStatus != enum.CommonYes || row.ProviderStatus != enum.CommonYes || row.ProviderDeleted != enum.CommonNo {
		return nil, errors.New("embedding provider model is not enabled")
	}
	apiKey, err := resolver.box.Decrypt(row.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	return resolver.factory.NewEmbeddingClient(ctx, infraai.EmbeddingClientConfig{EngineType: infraai.EngineType(row.EngineType), ModelKind: row.ModelKind,
		ModelID: row.ModelID, BaseURL: row.BaseURL, APIKey: apiKey, Capabilities: infraai.EmbeddingCapabilities{
			Dimensions: profile.EmbeddingDimensions, MaxInputs: documentEmbeddingBatch, MaxInputTokens: profile.EmbeddingMaxInputTokens,
			TokenCounterID: profile.EmbeddingTokenCounterID}})
}

var _ IngestionRepository = (*GormIngestionRepository)(nil)
var _ EmbeddingResolver = (*GormEmbeddingResolver)(nil)
