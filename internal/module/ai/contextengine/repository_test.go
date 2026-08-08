package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProviderModelOptionsQueryFiltersEnabledRowsAndKeepsStableOrder(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	repository := &GormAdminRepository{db: planRepository.db}

	mock.ExpectQuery(`(?s)SELECT .*pm\.id.*p\.name AS provider_name.*pm\.model_id.*pm\.model_kind.*pm\.display_name.*FROM .*ai_provider_models.*pm.*JOIN ai_providers AS p ON p\.id = pm\.provider_id.*WHERE pm\.status = \? AND p\.status = \? AND p\.is_del = \?.*ORDER BY p\.name ASC, pm\.model_id ASC, pm\.id ASC`).
		WithArgs(enum.CommonYes, enum.CommonYes, enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider_name", "model_id", "model_kind", "display_name"}).
			AddRow(uint64(11), "Alpha", "chat-v1", aiprovider.ModelKindChat, "Chat").
			AddRow(uint64(12), "Alpha", "embed-v1", aiprovider.ModelKindEmbedding, "Embedding").
			AddRow(uint64(13), "Beta", "rerank-v1", aiprovider.ModelKindRerank, ""))

	items, err := repository.ListProviderModelOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != 11 || items[1].ModelKind != aiprovider.ModelKindEmbedding || items[2].ProviderName != "Beta" {
		t.Fatalf("provider model options = %#v", items)
	}
	assertPlanMockExpectations(t, mock)
}

func TestFindDocumentVersionScopesVersionThroughDocumentAndSpace(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	repository := &GormAdminRepository{db: planRepository.db}

	mock.ExpectQuery(`(?s)SELECT v\.\* FROM ai_context_document_versions AS v JOIN ai_context_documents AS d ON d\.id = v\.document_id AND d\.deleted_at IS NULL JOIN ai_context_spaces AS s ON s\.id = d\.space_id AND s\.deleted_at IS NULL WHERE v\.id = \? AND s\.platform = \?.*LIMIT \?`).
		WithArgs(uint64(41), "admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "document_id", "profile_id", "source_storage_provider", "source_object_key", "source_etag", "source_size_bytes", "source_mime_type", "source_filename", "state",
		}).AddRow(uint64(41), uint64(31), uint64(21), "cos", "ai_context_documents/31/report.md", `"etag"`, int64(1024), "text/markdown", "report.md", "ready"))

	version, err := repository.FindDocumentVersion(context.Background(), "admin", 41)
	if err != nil {
		t.Fatal(err)
	}
	if version == nil || version.ID != 41 || version.DocumentID != 31 || version.SourceObjectKey != "ai_context_documents/31/report.md" {
		t.Fatalf("document version = %#v", version)
	}
	assertPlanMockExpectations(t, mock)
}

func TestContextPlanItemRowsRoundTripConversationTurnBoundaries(t *testing.T) {
	counter, err := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	turn := testConversationTurn()
	snapshot, err := BuildConversationTurnText(turn, counter, 4096)
	if err != nil {
		t.Fatal(err)
	}
	want := ContextConversationTurnV1{
		UserMessageID: turn.UserMessage.ID, AssistantMessageID: turn.AssistantMessage.ID,
		AttachmentContextByteOffset: snapshot.AttachmentContextByteOffset, ToolContextByteOffset: snapshot.ToolContextByteOffset,
		AssistantContextByteOffset: snapshot.AssistantContextByteOffset, AssistantDelivery: turn.AssistantDelivery,
	}
	item := ContextPlanItem{
		Ordinal: 1,
		Block: ContextBlock{
			Kind: BlockRecentTurn, SourceType: "conversation_turn", SourceRef: "conversation_turn:41",
			SourceSHA256: turn.SourceSHA256, AtomicGroupKey: "conversation_turn:41", Priority: 4,
			TokenUpperBound: snapshot.TokenUpperBound, ContentSnapshot: &snapshot.Text,
			Metadata: ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1, ConversationTurn: &want},
		},
		Decision: DecisionSelected,
	}

	rows, err := contextPlanItemRowsFromDomain(77, []ContextPlanItem{item})
	if err != nil {
		t.Fatal(err)
	}
	got, err := contextPlanItemsFromRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Block.Metadata.ConversationTurn == nil {
		t.Fatalf("round-tripped items = %#v", got)
	}
	if *got[0].Block.Metadata.ConversationTurn != want || got[0].Block.ContentSnapshot == nil || *got[0].Block.ContentSnapshot != snapshot.Text {
		t.Fatalf("round-tripped turn = %#v snapshot = %#v", got[0].Block.Metadata.ConversationTurn, got[0].Block.ContentSnapshot)
	}
}

func TestContextPlanItemsFromRowsAcceptLegacyMetadataWithoutTurnBoundaries(t *testing.T) {
	item := validReadyPlan().Items[0]
	rows, err := contextPlanItemRowsFromDomain(77, []ContextPlanItem{item})
	if err != nil {
		t.Fatal(err)
	}
	rows[0].MetadataJSON = `{"schema":"context_block_metadata_v1"}`

	got, err := contextPlanItemsFromRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Block.Metadata.ConversationTurn != nil {
		t.Fatalf("legacy metadata changed = %#v", got)
	}
}

func TestContextPlanRowsRoundTripDegradedPlan(t *testing.T) {
	want := degradedReadyPlan(t)
	want.ID = 77
	row, err := contextPlanRowFromDomain(want)
	if err != nil {
		t.Fatal(err)
	}
	row.ID = want.ID
	items, err := contextPlanItemRowsFromDomain(want.ID, want.Items)
	if err != nil {
		t.Fatal(err)
	}

	got, err := contextPlanFromRows(row, items)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.State != PlanReady || got.RetrievalOutcome != RetrievalDegraded || got.PlanSHA256 == nil || *got.PlanSHA256 != *want.PlanSHA256 {
		t.Fatalf("round-tripped degraded plan = %#v", got)
	}
	if got.Error == nil || got.Error.Stage != want.Error.Stage || got.Error.Code != want.Error.Code ||
		got.Error.Message == nil || want.Error.Message == nil || *got.Error.Message != *want.Error.Message {
		t.Fatalf("round-tripped degraded diagnostic = %#v, want %#v", got.Error, want.Error)
	}
}

func TestPersistTerminalRejectsInvalidInputBeforeTransaction(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	plan := validReadyPlan()

	if _, _, err := repository.PersistTerminal(t.Context(), plan, nil, validPlanCommitToken(plan)); !errors.Is(err, ErrNilPlanCommitGuard) {
		t.Fatalf("nil guard error = %v", err)
	}
	token := validPlanCommitToken(plan)
	token.InputFingerprintSHA256 = sha256.Sum256([]byte("other input"))
	if _, _, err := repository.PersistTerminal(t.Context(), plan, &recordingPlanGuard{}, token); !errors.Is(err, ErrInvalidPlanCommitToken) {
		t.Fatalf("snapshot token error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository opened a transaction for invalid input: %v", err)
	}
}

func TestProfileIndexCASIncludesGenerationFence(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	repository := &GormAdminRepository{db: planRepository.db}
	one, two := uint64(1), uint64(2)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_context_profiles` SET .* WHERE .*id = .*index_state = .*active_index_generation = .*target_index_generation IS NULL").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	ok, err := repository.CompareAndSwapProfileIndex(context.Background(), ProfileIndexCAS{
		ID:       7,
		Expected: ProfileIndex{State: ProfileIndexReady, ActiveGeneration: &one},
		Next:     ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: &one, TargetGeneration: &two},
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestAgentProfileChangeConflictRejectsOnlyReferencesFromAnotherProfile(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	repository := &GormAdminRepository{db: planRepository.db}

	mock.ExpectQuery(`(?s)SELECT\s+EXISTS\(SELECT 1 FROM ai_context_bindings b JOIN ai_context_spaces s.*s\.profile_id <> \?\)\s*\+\s*EXISTS\(SELECT 1 FROM ai_context_documents d JOIN ai_conversations c.*JOIN ai_context_document_versions v.*v\.profile_id <> \?\)\s*\+\s*EXISTS\(SELECT 1 FROM ai_conversation_memories m JOIN ai_conversations c.*m\.context_profile_id_snapshot <> \?\) AS ref_count`).
		WithArgs(uint64(7), uint64(11), uint64(7), uint64(11), uint64(7), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT\s+EXISTS\(SELECT 1 FROM ai_context_bindings b JOIN ai_context_spaces s.*s\.profile_id <> \?\)\s*\+\s*EXISTS\(SELECT 1 FROM ai_context_documents d JOIN ai_conversations c.*JOIN ai_context_document_versions v.*v\.profile_id <> \?\)\s*\+\s*EXISTS\(SELECT 1 FROM ai_conversation_memories m JOIN ai_conversations c.*m\.context_profile_id_snapshot <> \?\) AS ref_count`).
		WithArgs(uint64(7), uint64(12), uint64(7), uint64(12), uint64(7), uint64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"ref_count"}).AddRow(1))

	conflict, err := repository.AgentProfileChangeConflict(context.Background(), 7, 11)
	if err != nil || conflict {
		t.Fatalf("conflict=%v err=%v", conflict, err)
	}
	conflict, err = repository.AgentProfileChangeConflict(context.Background(), 7, 12)
	if err != nil || !conflict {
		t.Fatalf("different profile conflict=%v err=%v", conflict, err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestSetAgentContextProfileClearRemovesBindingsAtomicallyWhenAlreadyNull(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	repository := &GormAdminRepository{db: planRepository.db}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM ai_context_bindings WHERE agent_id = \?`).
		WithArgs(uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE `ai_agents` SET `context_profile_id`=\\? WHERE id = \\? AND is_del = \\?").
		WithArgs(nil, uint64(7), enum.CommonNo).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := repository.SetAgentContextProfile(context.Background(), 7, nil); err != nil {
		t.Fatal(err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistTerminalUsesSameTransactionAndPersistsSnapshotConflict(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	plan := validReadyPlan()
	guard := &recordingPlanGuard{result: PlanCommitGuardResult{SnapshotConflict: &PlanError{
		Stage: "authority", Code: ErrCodeSnapshotConflict,
	}}}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plans`")).
		WillReturnResult(sqlmock.NewResult(91, 1))
	mock.ExpectCommit()

	got, disposition, err := repository.PersistTerminal(t.Context(), plan, guard, validPlanCommitToken(plan))
	if err != nil {
		t.Fatal(err)
	}
	if disposition != PersistCreated || got.ID != 91 || got.State != PlanFailed || got.PlanSHA256 != nil || len(got.Items) != 0 {
		t.Fatalf("persisted conflict = %#v, disposition = %q", got, disposition)
	}
	if got.Error == nil || got.Error.Code != ErrCodeSnapshotConflict {
		t.Fatalf("persisted conflict error = %#v", got.Error)
	}
	if guard.transaction == nil || guard.transaction == repository.db {
		t.Fatal("guard did not receive the repository transaction")
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistTerminalDuplicateReloadsCommittedWinner(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	candidate := validReadyPlan()
	winner := candidate
	winner.ID = 73
	winnerHash := sha256.Sum256([]byte("winner"))
	winner.PlanSHA256 = &winnerHash

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plans`")).WillReturnError(&mysqldriver.MySQLError{
		Number: 1062, Message: "Duplicate entry '44' for key 'uk_ai_context_plans_run'",
	})
	mock.ExpectRollback()
	expectFindTerminalPlan(mock, winner)

	got, disposition, err := repository.PersistTerminal(t.Context(), candidate, &recordingPlanGuard{}, validPlanCommitToken(candidate))
	if err != nil {
		t.Fatal(err)
	}
	if disposition != PersistLoadedExisting || got.ID != winner.ID || got.PlanSHA256 == nil || *got.PlanSHA256 != winnerHash {
		t.Fatalf("reloaded plan = %#v, disposition = %q", got, disposition)
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistTerminalDuplicateReloadsFailedWinnerWithNullHash(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	candidate := validReadyPlan()
	winner := candidate
	winner.ID = 74
	winner.State = PlanFailed
	winner.RetrievalOutcome = RetrievalFailed
	winner.PlanSHA256 = nil
	winner.Items = nil
	winner.Error = &PlanError{Stage: "retrieval", Code: ErrCodeRetrievalFailed}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plans`")).WillReturnError(&mysqldriver.MySQLError{
		Number: 1062, Message: "Duplicate entry '44' for key 'uk_ai_context_plans_run'",
	})
	mock.ExpectRollback()
	expectFindTerminalPlan(mock, winner)

	got, disposition, err := repository.PersistTerminal(t.Context(), candidate, &recordingPlanGuard{}, validPlanCommitToken(candidate))
	if err != nil {
		t.Fatal(err)
	}
	if disposition != PersistLoadedExisting || got.ID != winner.ID || got.PlanSHA256 != nil || got.State != PlanFailed {
		t.Fatalf("reloaded failed plan = %#v, disposition = %q", got, disposition)
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistDispositionRejectsUnknownValue(t *testing.T) {
	if err := PersistCreated.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := PersistDisposition("unknown").Validate(); err == nil {
		t.Fatal("unknown persist disposition was accepted")
	}
}

func TestPersistTerminalConcurrentLoserReloadsWinner(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	mock.MatchExpectationsInOrder(false)
	left := validReadyPlan()
	right := validReadyPlan()
	right.InputFingerprintSHA256 = sha256.Sum256([]byte("right-input"))
	rightHash := sha256.Sum256([]byte("right"))
	right.PlanSHA256 = &rightHash
	winner := left
	winner.ID = 101

	mock.ExpectBegin()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plans`")).WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plans`")).
		WillDelayFor(20 * time.Millisecond).
		WillReturnError(&mysqldriver.MySQLError{Number: 1062, Message: "Duplicate entry '44' for key 'uk_ai_context_plans_run'"})
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `ai_context_plan_items`")).WillReturnResult(sqlmock.NewResult(201, 1))
	mock.ExpectCommit()
	mock.ExpectRollback()
	expectFindTerminalPlan(mock, winner)

	guard := &orderedPlanGuard{
		winnerInput: left.InputFingerprintSHA256,
		loserReady:  make(chan struct{}),
		winnerDone:  make(chan struct{}),
	}
	results := make(chan ContextPlan, 2)
	errorsChannel := make(chan error, 2)
	for _, plan := range []ContextPlan{left, right} {
		plan := plan
		go func() {
			got, _, err := repository.PersistTerminal(context.Background(), plan, guard, validPlanCommitToken(plan))
			if plan.InputFingerprintSHA256 == guard.winnerInput {
				close(guard.winnerDone)
			}
			results <- got
			errorsChannel <- err
		}()
	}
	<-guard.loserReady

	first, second := <-results, <-results
	if err := <-errorsChannel; err != nil {
		t.Fatal(err)
	}
	if err := <-errorsChannel; err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.ID != second.ID || first.PlanSHA256 == nil || second.PlanSHA256 == nil || *first.PlanSHA256 != *second.PlanSHA256 {
		t.Fatalf("concurrent callers returned different terminal plans: %#v %#v", first, second)
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistTerminalGuardAbortRollsBackWithoutPlan(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	plan := validReadyPlan()
	mock.ExpectBegin()
	mock.ExpectRollback()

	_, _, err := repository.PersistTerminal(t.Context(), plan, &recordingPlanGuard{err: ErrPlanCommitAborted}, validPlanCommitToken(plan))
	if !errors.Is(err, ErrPlanCommitAborted) {
		t.Fatalf("guard abort error = %v", err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestPersistTerminalContextCancellationIsCommitAbort(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	plan := validReadyPlan()
	mock.ExpectBegin()
	mock.ExpectRollback()

	_, _, err := repository.PersistTerminal(t.Context(), plan, &recordingPlanGuard{err: context.Canceled}, validPlanCommitToken(plan))
	if !errors.Is(err, ErrPlanCommitAborted) {
		t.Fatalf("context cancellation error = %v", err)
	}
	assertPlanMockExpectations(t, mock)
}

type recordingPlanGuard struct {
	transaction *gorm.DB
	result      PlanCommitGuardResult
	err         error
}

func (guard *recordingPlanGuard) GuardPlanCommitInTransaction(_ context.Context, transaction *gorm.DB, _ PlanCommitToken) (PlanCommitGuardResult, error) {
	guard.transaction = transaction
	return guard.result, guard.err
}

type orderedPlanGuard struct {
	once        sync.Once
	winnerInput [sha256.Size]byte
	loserReady  chan struct{}
	winnerDone  chan struct{}
}

func (guard *orderedPlanGuard) GuardPlanCommitInTransaction(_ context.Context, _ *gorm.DB, token PlanCommitToken) (PlanCommitGuardResult, error) {
	if token.InputFingerprintSHA256 == guard.winnerInput {
		return PlanCommitGuardResult{}, nil
	}
	guard.once.Do(func() { close(guard.loserReady) })
	<-guard.winnerDone
	return PlanCommitGuardResult{}, nil
}

func newPlanRepositoryFixture(t *testing.T) (*GormPlanRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: false,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return NewPlanRepository(&database.Client{Gorm: db, SQL: sqlDB}), mock, func() { _ = sqlDB.Close() }
}

func validPlanCommitToken(plan ContextPlan) PlanCommitToken {
	return PlanCommitToken{
		RunID: plan.RunID, ReplyCommandID: 77, LeaseOwner: "worker-a", LeaseToken: 3,
		InputFingerprintSHA256:  plan.InputFingerprintSHA256,
		AuthoritySnapshotSHA256: sha256.Sum256([]byte("authority")),
	}
}

func expectFindTerminalPlan(mock sqlmock.Sqlmock, plan ContextPlan) {
	profileID, profileHash, generation := any(nil), any(nil), any(nil)
	if plan.Profile != nil {
		profileID, profileHash, generation = plan.Profile.ID, plan.Profile.SHA256[:], plan.Profile.IndexGeneration
	}
	planHash := any(nil)
	if plan.PlanSHA256 != nil {
		planHash = plan.PlanSHA256[:]
	}
	errorStage, errorCode, errorMessage := any(nil), any(nil), any(nil)
	if plan.Error != nil {
		errorStage, errorCode = plan.Error.Stage, string(plan.Error.Code)
		if plan.Error.Message != nil {
			errorMessage = *plan.Error.Message
		}
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_plans` WHERE run_id = ? ORDER BY id ASC LIMIT ?")).
		WithArgs(plan.RunID, 1).
		WillReturnRows(sqlmock.NewRows(contextPlanColumnNames()).AddRow(
			plan.ID, plan.RunID, profileID, profileHash, generation, plan.PolicyVersion,
			plan.InputFingerprintSHA256[:], planHash, plan.ModelCapabilitySHA256[:],
			plan.APIProtocol, plan.TokenCounterID, plan.Budget.ContextWindowTokens,
			plan.Budget.EffectiveOutputTokens, plan.Budget.ProviderProtocolUpperBound,
			plan.Budget.ToolContinuationInputReserve, plan.Budget.PolicySafetyMargin,
			plan.Budget.KnownInputBudget, plan.Budget.KnownInputUpperBound, plan.Budget.Proof,
			plan.RetrievalOutcome, plan.State, errorStage, errorCode, errorMessage,
			`{"schema":"context_plan_metrics_v1"}`, time.Now(),
		))
	if len(plan.Items) == 0 {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_plan_items` WHERE plan_id = ? ORDER BY ordinal ASC")).
			WithArgs(plan.ID).
			WillReturnRows(sqlmock.NewRows(contextPlanItemColumnNames()))
		return
	}
	item := plan.Items[0]
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_plan_items` WHERE plan_id = ? ORDER BY ordinal ASC")).
		WithArgs(plan.ID).
		WillReturnRows(sqlmock.NewRows(contextPlanItemColumnNames()).AddRow(
			201, plan.ID, item.Ordinal, item.Block.Kind, item.Block.SourceType, item.Block.SourceRef,
			item.Block.SourceSHA256[:], item.Block.AtomicGroupKey, 1, item.Block.Priority, item.Decision,
			nil, item.Block.TokenUpperBound, nil, nil, nil, *item.Block.ContentSnapshot,
			`{"schema":"context_block_metadata_v1"}`, time.Now(),
		))
}

func assertPlanMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
