package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrCandidateAuthorityNotConfigured = errors.New("context candidate authority is not configured")

type CandidateAuthoritySnapshot struct {
	ProfileID       uint64
	IndexGeneration uint64
	AgentID         uint64
	UserID          uint64
	ConversationID  uint64
	Platform        string
}

func (snapshot CandidateAuthoritySnapshot) Validate() error {
	if snapshot.ProfileID == 0 || snapshot.IndexGeneration == 0 || snapshot.AgentID == 0 || snapshot.UserID == 0 ||
		snapshot.ConversationID == 0 || strings.TrimSpace(snapshot.Platform) == "" || strings.TrimSpace(snapshot.Platform) != snapshot.Platform {
		return ErrInvalidContextPlan
	}
	return nil
}

type CandidateVerification struct {
	Authorized []VerifiedCandidate
	Excluded   []CandidateExclusion
	Cleanup    []contextindex.PointRef
}

type CandidateAuthorityReader interface {
	VerifyCandidates(context.Context, CandidateAuthoritySnapshot, []Candidate) (CandidateVerification, error)
}

type GormCandidateRepository struct {
	db           *gorm.DB
	conversation ConversationTurnReader
}

func NewCandidateRepository(client *database.Client) *GormCandidateRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormCandidateRepository{db: client.Gorm, conversation: NewConversationRepository(client)}
}

func NewCandidateRepositoryWithDB(db *gorm.DB, conversation ConversationTurnReader) *GormCandidateRepository {
	if db == nil || conversation == nil {
		return nil
	}
	return &GormCandidateRepository{db: db, conversation: conversation}
}

type candidateDocumentRow struct {
	ChunkID                       uint64  `gorm:"column:chunk_id"`
	DocumentVersionID             uint64  `gorm:"column:document_version_id"`
	Ordinal                       uint32  `gorm:"column:ordinal"`
	Content                       string  `gorm:"column:content"`
	ContentSHA256                 []byte  `gorm:"column:content_sha256"`
	ChunkFactsSHA256              []byte  `gorm:"column:chunk_facts_sha256"`
	EmbeddingInputTokenUpperBound uint64  `gorm:"column:embedding_input_token_upper_bound"`
	LocatorJSON                   string  `gorm:"column:locator_json"`
	VersionProfileID              uint64  `gorm:"column:version_profile_id"`
	VersionState                  string  `gorm:"column:version_state"`
	DocumentID                    uint64  `gorm:"column:document_id"`
	DocumentTitle                 string  `gorm:"column:document_title"`
	DocumentSpaceID               *uint64 `gorm:"column:document_space_id"`
	DocumentConversationID        *uint64 `gorm:"column:document_conversation_id"`
	DocumentActiveVersionID       *uint64 `gorm:"column:document_active_version_id"`
	DocumentStatus                string  `gorm:"column:document_status"`
	SpaceProfileID                *uint64 `gorm:"column:space_profile_id"`
	SpacePlatform                 *string `gorm:"column:space_platform"`
	SpaceStatus                   *string `gorm:"column:space_status"`
	BindingStatus                 *string `gorm:"column:binding_status"`
	ConversationUserID            *uint64 `gorm:"column:conversation_user_id"`
}

func (repository *GormCandidateRepository) VerifyCandidates(
	ctx context.Context,
	snapshot CandidateAuthoritySnapshot,
	candidates []Candidate,
) (CandidateVerification, error) {
	if repository == nil || repository.db == nil || repository.conversation == nil {
		return CandidateVerification{}, ErrCandidateAuthorityNotConfigured
	}
	if err := snapshot.Validate(); err != nil {
		return CandidateVerification{}, err
	}
	if len(candidates) == 0 {
		return CandidateVerification{}, nil
	}
	var profile ContextProfile
	if err := repository.db.WithContext(ctx).Where("id = ?", snapshot.ProfileID).Take(&profile).Error; err != nil {
		return CandidateVerification{}, err
	}
	if profile.Status != ProfileEnabled || profile.ActiveIndexGeneration == nil || *profile.ActiveIndexGeneration != snapshot.IndexGeneration ||
		(profile.IndexState != ProfileIndexReady && profile.IndexState != ProfileIndexRebuilding) {
		return CandidateVerification{}, ErrIndexGenerationUnavailable
	}
	counter, err := infraai.ResolveTokenCounter(profile.EmbeddingTokenCounterID)
	if err != nil {
		return CandidateVerification{}, err
	}

	documentIDs := make([]uint64, 0, len(candidates))
	turnAnchors := make([]uint64, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.Point.SourceKind {
		case contextindex.SourceKindDocumentChunk:
			documentIDs = append(documentIDs, candidate.Point.SourceID)
		case contextindex.SourceKindConversationTurn:
			turnAnchors = append(turnAnchors, candidate.Point.SourceID)
		default:
			return CandidateVerification{}, fmt.Errorf("%w: unsupported candidate source kind %q", ErrInvalidContextPlan, candidate.Point.SourceKind)
		}
	}

	documentRows, err := repository.loadDocumentCandidates(ctx, snapshot.AgentID, documentIDs)
	if err != nil {
		return CandidateVerification{}, err
	}
	var turns []ConversationTurn
	if len(turnAnchors) != 0 {
		turns, err = repository.conversation.CompleteByAnchors(ctx, snapshot.ConversationID, snapshot.UserID, uniqueUint64(turnAnchors))
		if err != nil {
			return CandidateVerification{}, err
		}
	}
	documentsByID := make(map[uint64]candidateDocumentRow, len(documentRows))
	for _, row := range documentRows {
		if _, duplicate := documentsByID[row.ChunkID]; duplicate {
			return CandidateVerification{}, fmt.Errorf("%s: duplicate authoritative chunk row", ErrCodeIndexInconsistent)
		}
		documentsByID[row.ChunkID] = row
	}
	turnsByAnchor := make(map[uint64]ConversationTurn, len(turns))
	for _, turn := range turns {
		turnsByAnchor[turn.UserMessage.ID] = turn
	}

	result := CandidateVerification{Authorized: make([]VerifiedCandidate, 0, len(candidates))}
	for _, candidate := range candidates {
		if candidate.Point.ProfileID != snapshot.ProfileID || candidate.Point.IndexGeneration != snapshot.IndexGeneration {
			result.excludeStale(candidate, ExclusionInactiveSource)
			continue
		}
		switch candidate.Point.SourceKind {
		case contextindex.SourceKindDocumentChunk:
			verified, ok, verifyErr := verifyDocumentCandidate(snapshot, candidate, documentsByID[candidate.Point.SourceID])
			if verifyErr != nil {
				return CandidateVerification{}, verifyErr
			}
			if !ok {
				result.excludeStale(candidate, ExclusionPermissionChanged)
				continue
			}
			result.Authorized = append(result.Authorized, verified)
		case contextindex.SourceKindConversationTurn:
			turn, ok := turnsByAnchor[candidate.Point.SourceID]
			if !ok || turn.SourceSHA256 != candidate.Point.SourceSHA256 {
				result.excludeStale(candidate, ExclusionInactiveSource)
				continue
			}
			text, textErr := BuildConversationTurnText(turn, counter, profile.EmbeddingMaxInputTokens)
			if textErr != nil {
				return CandidateVerification{}, textErr
			}
			turnCopy := turn
			result.Authorized = append(result.Authorized, VerifiedCandidate{
				Candidate: candidate, SourceType: "conversation_turn", SourceSHA256: turn.SourceSHA256,
				ContentSHA256: sha256.Sum256([]byte(text.Text)), Content: text.Text, TokenUpperBound: text.TokenUpperBound,
				ConversationTurn: &turnCopy,
			})
		}
	}
	return result, nil
}

func (repository *GormCandidateRepository) loadDocumentCandidates(ctx context.Context, agentID uint64, chunkIDs []uint64) ([]candidateDocumentRow, error) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	var rows []candidateDocumentRow
	err := repository.db.WithContext(ctx).Table("ai_context_chunks AS chunk").Select(`
		chunk.id AS chunk_id, chunk.document_version_id, chunk.ordinal, chunk.content, chunk.content_sha256,
		chunk.chunk_facts_sha256, chunk.embedding_input_token_upper_bound, chunk.locator_json,
		version.profile_id AS version_profile_id, version.state AS version_state,
		document.id AS document_id, document.title AS document_title, document.space_id AS document_space_id,
		document.conversation_id AS document_conversation_id, document.active_version_id AS document_active_version_id,
		document.status AS document_status, space.profile_id AS space_profile_id, space.platform AS space_platform,
		space.status AS space_status, binding.status AS binding_status, conversation.user_id AS conversation_user_id`).
		Joins("JOIN ai_context_document_versions AS version ON version.id = chunk.document_version_id").
		Joins("JOIN ai_context_documents AS document ON document.id = version.document_id AND document.deleted_at IS NULL").
		Joins("LEFT JOIN ai_context_spaces AS space ON space.id = document.space_id AND space.deleted_at IS NULL").
		Joins("LEFT JOIN ai_context_bindings AS binding ON binding.space_id = space.id AND binding.agent_id = ?", agentID).
		Joins("LEFT JOIN ai_conversations AS conversation ON conversation.id = document.conversation_id AND conversation.is_del = ?", enum.CommonNo).
		Where("chunk.id IN ?", uniqueUint64(chunkIDs)).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := rows[:0]
	for _, row := range rows {
		if row.DocumentConversationID != nil {
			authoritative, err := conversationDocumentVersionAuthoritative(ctx, repository.db, row.DocumentVersionID, true)
			if err != nil {
				return nil, err
			}
			if !authoritative {
				continue
			}
		}
		result = append(result, row)
	}
	return result, nil
}

func verifyDocumentCandidate(snapshot CandidateAuthoritySnapshot, candidate Candidate, row candidateDocumentRow) (VerifiedCandidate, bool, error) {
	if row.ChunkID == 0 {
		return VerifiedCandidate{}, false, nil
	}
	contentHash, err := SHA256FromBytes(row.ContentSHA256)
	if err != nil || contentHash != sha256.Sum256([]byte(row.Content)) {
		return VerifiedCandidate{}, false, fmt.Errorf("%s: document content hash disagrees", ErrCodeIndexInconsistent)
	}
	factsHash, err := SHA256FromBytes(row.ChunkFactsSHA256)
	if err != nil {
		return VerifiedCandidate{}, false, fmt.Errorf("%s: document facts hash is invalid", ErrCodeIndexInconsistent)
	}
	if factsHash != candidate.Point.SourceSHA256 || row.VersionProfileID != snapshot.ProfileID || row.VersionState != DocumentVersionReady ||
		row.DocumentActiveVersionID == nil || *row.DocumentActiveVersionID != row.DocumentVersionID || row.DocumentStatus != DocumentEnabled {
		return VerifiedCandidate{}, false, nil
	}
	spaceAuthorized := row.DocumentSpaceID != nil && row.SpaceProfileID != nil && *row.SpaceProfileID == snapshot.ProfileID &&
		row.SpacePlatform != nil && *row.SpacePlatform == snapshot.Platform && row.SpaceStatus != nil && *row.SpaceStatus == SpaceEnabled &&
		row.BindingStatus != nil && *row.BindingStatus == "enabled"
	conversationAuthorized := row.DocumentConversationID != nil && *row.DocumentConversationID == snapshot.ConversationID &&
		row.ConversationUserID != nil && *row.ConversationUserID == snapshot.UserID
	if !spaceAuthorized && !conversationAuthorized {
		return VerifiedCandidate{}, false, nil
	}
	var locator ContextLocatorV1
	if err := json.Unmarshal([]byte(row.LocatorJSON), &locator); err != nil || locator.Validate() != nil || row.EmbeddingInputTokenUpperBound == 0 {
		return VerifiedCandidate{}, false, fmt.Errorf("%s: document locator or token facts are invalid", ErrCodeIndexInconsistent)
	}
	return VerifiedCandidate{
		Candidate: candidate, SourceType: "document_chunk", SourceSHA256: factsHash,
		Title: row.DocumentTitle, DocumentID: row.DocumentID, DocumentVersionID: row.DocumentVersionID,
		ChunkIDs: []uint64{row.ChunkID}, ChunkOrdinals: []uint32{row.Ordinal}, ChunkFactsSHA256: [][sha256.Size]byte{factsHash},
		ContentSHA256: contentHash, Content: row.Content, TokenUpperBound: int64(row.EmbeddingInputTokenUpperBound), Locators: []ContextLocatorV1{locator},
	}, true, nil
}

func (result *CandidateVerification) excludeStale(candidate Candidate, reason ExclusionReason) {
	result.Excluded = append(result.Excluded, CandidateExclusion{Candidate: candidate, Reason: reason})
	result.Cleanup = append(result.Cleanup, candidate.Point)
}

func uniqueUint64(input []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(input))
	result := make([]uint64, 0, len(input))
	for _, value := range input {
		if value == 0 {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
