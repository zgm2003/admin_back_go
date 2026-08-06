package contextengine

import (
	"context"
	"crypto/sha256"
	"regexp"
	"testing"

	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCandidateAuthorityUsesFixedProfileAndDocumentBatches(t *testing.T) {
	db, mock, closeDB := newCandidateRepositoryFixture(t)
	defer closeDB()
	facts := sha256.Sum256([]byte("facts"))
	content := "refunds take three days"
	contentHash := sha256.Sum256([]byte(content))
	active := uint64(3)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_profiles` WHERE id = ? LIMIT ?")).
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "embedding_max_input_tokens", "embedding_token_counter_id", "status", "active_index_generation", "index_state"}).
			AddRow(uint64(7), int64(4096), "utf8_bytes_v1", ProfileEnabled, active, ProfileIndexReady))
	mock.ExpectQuery(`SELECT .* FROM ai_context_chunks AS chunk .* LEFT JOIN ai_context_bindings AS binding .* WHERE chunk.id IN \(\?\)`).
		WithArgs(uint64(9), enum.CommonNo, uint64(41)).
		WillReturnRows(sqlmock.NewRows([]string{
			"chunk_id", "document_version_id", "ordinal", "content", "content_sha256", "chunk_facts_sha256", "embedding_input_token_upper_bound", "locator_json",
			"version_profile_id", "version_state", "document_id", "document_title", "document_space_id", "document_conversation_id", "document_active_version_id",
			"document_status", "space_profile_id", "space_platform", "space_status", "binding_status", "conversation_user_id",
		}).AddRow(uint64(41), uint64(31), uint32(0), content, contentHash[:], facts[:], uint64(64), `{"schema":"context_locator_v1","kind":"paragraph","paragraph":1}`,
			uint64(7), DocumentVersionReady, uint64(21), "Refund policy", uint64(11), nil, uint64(31), DocumentEnabled,
			uint64(7), "admin", SpaceEnabled, "enabled", nil))

	score, _ := ParseFixedScore("0.900000")
	ref := contextindex.PointRef{ID: uuid.MustParse("80000000-0000-8000-8000-000000000041"), ProfileID: 7, IndexGeneration: 3, SourceKind: contextindex.SourceKindDocumentChunk, SourceID: 41, SourceSHA256: facts}
	verification, err := db.VerifyCandidates(context.Background(), CandidateAuthoritySnapshot{ProfileID: 7, IndexGeneration: 3, AgentID: 9, UserID: 5, ConversationID: 4, Platform: "admin"}, []Candidate{{Point: ref, FusionScore: score, Branches: RetrievalBranchesV1{Schema: RetrievalBranchesSchemaV1, Branches: []RetrievalBranchV1{{VariantID: "current", Modality: "dense", Rank: 1, Score: score}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(verification.Authorized) != 1 || len(verification.Excluded) != 0 || verification.Authorized[0].Content != content {
		t.Fatalf("verification=%+v", verification)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateAuthorityWithoutConversationIdentityRejectsConversationTurns(t *testing.T) {
	repository, mock, closeDB := newCandidateRepositoryFixtureWithTurns(t, &trackingConversationTurnReader{})
	defer closeDB()
	active := uint64(3)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_profiles` WHERE id = ? LIMIT ?")).
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "embedding_max_input_tokens", "embedding_token_counter_id", "status", "active_index_generation", "index_state"}).
			AddRow(uint64(7), int64(4096), "utf8_bytes_v1", ProfileEnabled, active, ProfileIndexReady))

	reader := repository.conversation.(*trackingConversationTurnReader)
	score, _ := ParseFixedScore("0.900000")
	ref := contextindex.PointRef{
		ID: uuid.MustParse("80000000-0000-8000-8000-000000000051"), ProfileID: 7, IndexGeneration: 3,
		SourceKind: contextindex.SourceKindConversationTurn, SourceID: 51, SourceSHA256: sha256.Sum256([]byte("turn facts")),
	}
	verification, err := repository.VerifyCandidates(context.Background(), CandidateAuthoritySnapshot{
		ProfileID: 7, IndexGeneration: 3, AgentID: 9, Platform: "admin",
	}, []Candidate{{Point: ref, FusionScore: score}})
	if err != nil {
		t.Fatal(err)
	}
	if reader.completeCalls != 0 || len(verification.Authorized) != 0 || len(verification.Excluded) != 1 || len(verification.Cleanup) != 0 ||
		verification.Excluded[0].Reason != ExclusionPermissionChanged {
		t.Fatalf("reader calls=%d verification=%+v", reader.completeCalls, verification)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateAuthoritySnapshotRequiresCompleteOrAbsentConversationIdentity(t *testing.T) {
	base := CandidateAuthoritySnapshot{ProfileID: 7, IndexGeneration: 3, AgentID: 9, Platform: "admin"}
	if err := base.Validate(); err != nil {
		t.Fatalf("admin evaluation snapshot: %v", err)
	}
	base.UserID = 5
	if err := base.Validate(); err == nil {
		t.Fatal("partial conversation identity must be rejected")
	}
	base.ConversationID = 4
	if err := base.Validate(); err != nil {
		t.Fatalf("chat snapshot: %v", err)
	}
}

type emptyConversationTurnReader struct{}

func (emptyConversationTurnReader) NewestComplete(context.Context, uint64, uint64, *uint64) (*ConversationTurn, error) {
	return nil, nil
}

func (emptyConversationTurnReader) CompleteByAnchors(context.Context, uint64, uint64, []uint64) ([]ConversationTurn, error) {
	return nil, nil
}

type trackingConversationTurnReader struct{ completeCalls int }

func (*trackingConversationTurnReader) NewestComplete(context.Context, uint64, uint64, *uint64) (*ConversationTurn, error) {
	return nil, nil
}

func (reader *trackingConversationTurnReader) CompleteByAnchors(context.Context, uint64, uint64, []uint64) ([]ConversationTurn, error) {
	reader.completeCalls++
	return nil, nil
}

func newCandidateRepositoryFixture(t *testing.T) (*GormCandidateRepository, sqlmock.Sqlmock, func()) {
	return newCandidateRepositoryFixtureWithTurns(t, emptyConversationTurnReader{})
}

func newCandidateRepositoryFixtureWithTurns(t *testing.T, turns ConversationTurnReader) (*GormCandidateRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return NewCandidateRepositoryWithDB(db, turns), mock, func() { _ = sqlDB.Close() }
}
