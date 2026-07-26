package replycommand

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/requestidentity"

	"github.com/DATA-DOG/go-sqlmock"
)

const durableWorkIntegrationEnv = "ADMIN_DURABLE_WORK_INTEGRATION"

func TestIdempotencyKeyIsStableAndUserScoped(t *testing.T) {
	first := idempotencyKey(31, "request-7")
	if len(first) != 64 || first != idempotencyKey(31, "request-7") {
		t.Fatalf("unstable key %q", first)
	}
	if first == idempotencyKey(32, "request-7") || first == idempotencyKey(31, "request-8") {
		t.Fatal("idempotency key did not scope user and request")
	}
}

func TestCreateReplyReplayLocksCanonicalCommandBeforeConversation(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := testCreateReplyInput(3, 7, "request-1", "hello")
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WithArgs(int64(7), "request-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "request_fingerprint", "request_identity_status", "user_message_id", "state"}).
			AddRow(41, "request-1", input.RequestFingerprint[:], requestidentity.IdentityStatusReplayable, 51, StatePending))
	mock.ExpectQuery("SELECT .* FROM `ai_runs`").
		WithArgs(int64(7), "request-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(61))
	mock.ExpectQuery("SELECT .* FROM `ai_usage_charges`").
		WithArgs(int64(61), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(71))
	mock.ExpectCommit()

	result, err := repository.CreateReply(context.Background(), input)
	if err != nil || result.CommandID != 41 || result.UserMessageID != 51 || result.RunID != 61 || result.ChargeID != 71 {
		t.Fatalf("replay=%+v err=%v now=%v", result, err, now)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReplyRejectsNonEmptyIdentityMarkerBeforeDatabaseAccess(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	input := testCreateReplyInput(3, 7, "request-1", "hello")
	input.RequestIdentityMarker = " "

	if _, err := repository.CreateReply(context.Background(), input); !errors.Is(err, ErrCreateInputInvalid) {
		t.Fatalf("whitespace request identity marker error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTransitionRejectsNonCanonicalMachineCodeBeforeDatabaseAccess(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()

	ok, err := repository.Transition(context.Background(), 41, "worker-a", 1, StateRunning, StateFailed, map[string]any{
		"finished_at": time.Now(), "last_error_code": " ai.reply_failed ", "last_error_message": "provider failed",
	})
	if ok || !errors.Is(err, ErrCreateInputInvalid) {
		t.Fatalf("transition ok=%v err=%v", ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRenewExtendsLeaseAfterDurableCancellation(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	canceledAt := now.Add(-time.Second)

	mock.ExpectBegin()
	mock.ExpectExec(`^UPDATE `+"`ai_reply_commands`"+` SET `+"`lease_expires_at`"+`=\?,`+"`updated_at`"+`=\? WHERE id = \? AND lease_owner = \? AND lease_token = \? AND state IN \(\?,\?\)$`).
		WithArgs(now.Add(time.Minute), sqlmock.AnyArg(), uint64(41), "worker-a", uint64(7), StateClaimed, StateRunning).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").
		WithArgs(uint64(41), "worker-a", uint64(7), StateClaimed, StateRunning, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cancel_requested_at"}).AddRow(41, canceledAt))
	mock.ExpectCommit()

	renewal, err := repository.Renew(context.Background(), 41, "worker-a", 7, now.Add(time.Minute))
	if err != nil || !renewal.Alive || !renewal.CancelRequested {
		t.Fatalf("renewal=%+v err=%v", renewal, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimFinalizationRetryAtMaxDoesNotIncrementProviderAttempts(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "request_id", "user_id", "conversation_id", "user_message_id", "state", "attempt_count", "max_attempts", "lease_token", "next_attempt_at", "last_error_code", "last_error_message",
	}).AddRow(41, "finalization-retry", 7, 3, 9, StatePending, 3, 3, 8, now.Add(-time.Second), ErrCodeFinalizationRetry, FinalizationRetryMarker))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(sqlmock.NewRows([]string{"id", "command_id", "attempt_no", "state"}).AddRow(90, 41, 3, AttemptFailed))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claim, err := repository.ClaimNext(context.Background(), "worker-b", now, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if claim.Command.AttemptCount != 3 || claim.Command.MaxAttempts != 3 || claim.FencingToken != 9 {
		t.Fatalf("finalization-only claim consumed provider attempt: %+v", claim)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleFinalizationRetryAddsStableMarkerWhenNoTriggerExists(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{"id", "state", "lease_owner", "lease_token", "last_error_code", "last_error_message"}).AddRow(41, StateRunning, "worker-a", 7, "", ""))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repository.ScheduleFinalizationRetry(context.Background(), 41, "worker-a", 7, now, now.Add(time.Second))
	if err != nil || !ok {
		t.Fatalf("scheduled=%v err=%v", ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimProviderFailureFinalizationMarkerAtMaxDoesNotIncrementProviderAttempts(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "request_id", "user_id", "conversation_id", "user_message_id", "state", "attempt_count", "max_attempts", "lease_token", "next_attempt_at", "last_error_code", "last_error_message",
	}).AddRow(42, "provider-finalization", 7, 3, 9, StatePending, 3, 3, 8, now.Add(-time.Second), "ai.provider_failed", "provider_failed"))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(sqlmock.NewRows([]string{"id", "command_id", "attempt_no", "state"}).AddRow(90, 42, 3, AttemptFailed))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claim, err := repository.ClaimNext(context.Background(), "worker-b", now, time.Minute)
	if err != nil || claim == nil || claim.Command.AttemptCount != 3 || claim.Command.LastErrorCode != "ai.provider_failed" || claim.Command.LastErrorMessage != "provider_failed" {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimExpiredSucceededAttemptPreservesCandidateWithoutGenericMarker(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "request_id", "user_id", "conversation_id", "user_message_id", "state", "attempt_count", "max_attempts", "lease_token", "lease_expires_at", "next_attempt_at", "last_error_code", "last_error_message",
	}).AddRow(43, "succeeded-finalization", 7, 3, 9, StateRunning, 3, 3, 8, expired, now.Add(-time.Second), "", ""))
	candidate := `{"version":"ai_chat_result_v1","tool_calls":[{"id":"call-1","name":"lookup"}]}`
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(sqlmock.NewRows([]string{"id", "command_id", "attempt_no", "state", "usage_json", "result_candidate_json"}).AddRow(91, 43, 3, AttemptSucceeded, `{"status":"reported"}`, candidate))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claim, err := repository.ClaimNext(context.Background(), "worker-b", now, time.Minute)
	if err != nil || claim == nil || claim.Command.AttemptCount != 3 || claim.Command.LastErrorCode != "" || claim.Command.LastErrorMessage != "" {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimPendingPreparedAttemptBelowMaxReusesProviderAttemptNumber(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "request_id", "user_id", "conversation_id", "user_message_id", "state", "attempt_count", "max_attempts", "lease_token", "next_attempt_at",
	}).AddRow(45, "prepared-below-max", 7, 3, 9, StatePending, 1, 3, 8, now.Add(-time.Second)))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(sqlmock.NewRows([]string{"id", "command_id", "attempt_no", "state"}).AddRow(93, 45, 1, AttemptPrepared))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claim, err := repository.ClaimNext(context.Background(), "worker-b", now, time.Minute)
	if err != nil || claim == nil || claim.Command.AttemptCount != 1 {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimPendingPreparedAttemptAtMaxReusesProviderAttemptNumber(t *testing.T) {
	repository, _, mock, closeDB := newAttemptMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "request_id", "user_id", "conversation_id", "user_message_id", "state", "attempt_count", "max_attempts", "lease_token", "next_attempt_at",
	}).AddRow(44, "prepared-recovery", 7, 3, 9, StatePending, 3, 3, 8, now.Add(-time.Second)))
	mock.ExpectQuery("SELECT .* FROM `ai_provider_attempts`").WillReturnRows(sqlmock.NewRows([]string{"id", "command_id", "attempt_no", "state"}).AddRow(92, 44, 3, AttemptPrepared))
	mock.ExpectExec("UPDATE `ai_reply_commands` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claim, err := repository.ClaimNext(context.Background(), "worker-b", now, time.Minute)
	if err != nil || claim == nil || claim.Command.AttemptCount != 3 {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReplyRollsBackFailuresAndReturnsOriginalDuplicate(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()

	invalidJSON := "{"
	invalidInput := testCreateReplyInput(fixture.conversationID, fixture.userID, "message-insert-failure", "message must roll back")
	invalidInput.MetaJSON = &invalidJSON
	_, err := repository.CreateReply(ctx, invalidInput)
	if err == nil {
		t.Fatal("expected invalid message JSON to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, "message-insert-failure", "message must roll back", 0, 0)

	maxRequestID := strings.Repeat("界", 128)
	if _, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, maxRequestID, "128 character request ID")); err != nil {
		t.Fatalf("128-character request_id was rejected: %v", err)
	}
	assertReplyRows(t, db, fixture.conversationID, maxRequestID, "128 character request ID", 1, 1)

	tooLongRequestID := strings.Repeat("界", 129)
	_, err = repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, tooLongRequestID, "command failure must roll back message"))
	if err == nil {
		t.Fatal("expected oversized command request_id to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, tooLongRequestID, "command failure must roll back message", 0, 0)

	collisionKey := fmt.Sprintf("p05-collision-%d", time.Now().UnixNano())
	insertCommandKeyBlocker(t, db, fixture, collisionKey)
	repository.idempotencyKey = func(int64, string) string { return collisionKey }
	_, err = repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "command-insert-failure", "unique command failure must roll back message"))
	if err == nil {
		t.Fatal("expected command unique identity conflict to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, "command-insert-failure", "unique command failure must roll back message", 0, 0)
	repository.idempotencyKey = idempotencyKey

	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "request-1", "original content"))
	if err != nil {
		t.Fatalf("CreateReply: %v", err)
	}
	if created.CommandID == 0 || created.UserMessageID == 0 || created.RequestID != "request-1" || created.State != StatePending {
		t.Fatalf("unexpected create result: %+v", created)
	}

	duplicate, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "request-1", "original content"))
	if err != nil {
		t.Fatalf("duplicate CreateReply: %v", err)
	}
	if duplicate.CommandID != created.CommandID || duplicate.UserMessageID != created.UserMessageID || duplicate.State != StatePending {
		t.Fatalf("duplicate did not return original identity: created=%+v duplicate=%+v", created, duplicate)
	}
	assertReplyRows(t, db, fixture.conversationID, "request-1", "original content", 1, 1)
	mismatch := testCreateReplyInput(fixture.conversationID, fixture.userID, "request-1", "different content")
	if _, err := repository.CreateReply(ctx, mismatch); !errors.Is(err, requestidentity.ErrRequestIdentityConflict) {
		t.Fatalf("mismatched replay error=%v", err)
	}
	assertReplyRows(t, db, fixture.conversationID, "request-1", "different content", 1, 0)
}

func testCreateReplyInput(conversationID, userID int64, requestID, content string) CreateReplyInput {
	fingerprint, err := requestidentity.BuildChatFingerprint(requestidentity.ChatFingerprintInput{
		UserID: userID, ConversationID: conversationID, AgentID: 1, ModelID: "test-model", Text: content, Options: requestidentity.GenerationOptions{MaxOutputTokens: 4096},
	})
	if err != nil {
		panic(err)
	}
	return CreateReplyInput{
		ConversationID: conversationID, UserID: userID, AgentID: 1, ProviderID: 2,
		ModelID: "test-model", ModelDisplayName: "Test Model", RequestID: requestID, Content: content, InputSnapshot: content,
		PricingSnapshotJSON: `{"version":"test-v1","billable":true,"catalog_vendor":"test-vendor","transport_engine":"openai","requested_model_id":"test-model","canonical_model_id":"test-model","catalog_max_output_tokens":8192,"effective_max_output_tokens":4096,"multiplier_ppm":1000000,"source_url":"https://example.test/pricing","retrieved_at":"2026-07-25","rates":[{"category":"input","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":1,"unit_scale":1000000}]}`,
		EffectiveMaxTokens:  4096, RequestFingerprint: fingerprint, RequestIdentityStatus: requestidentity.IdentityStatusReplayable,
	}
}

type replyFixture struct {
	userID         int64
	agentID        int64
	conversationID int64
}

func openReplyIntegrationDB(t *testing.T) *database.Client {
	t.Helper()
	if os.Getenv(durableWorkIntegrationEnv) != "1" {
		t.Skip("Docker-only durable work integration test")
	}
	db, err := database.Open(config.MySQLConfig{
		DSN:             os.Getenv("MYSQL_DSN"),
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open integration MySQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createReplyFixture(t *testing.T, db *database.Client) replyFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	insert := func(query string, args ...any) int64 {
		result, err := db.SQL.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("insert reply fixture: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("reply fixture id: %v", err)
		}
		return id
	}
	userID := insert("INSERT INTO users (username, status, is_del) VALUES (?, 1, 2)", "p05-reply-"+suffix)
	agentID := insert("INSERT INTO ai_agents (provider_id, name, scenes_json, status, is_del) VALUES (0, ?, JSON_ARRAY('chat'), 1, 2)", "p05-reply-"+suffix)
	conversationID := insert("INSERT INTO ai_conversations (user_id, agent_id, title, is_del) VALUES (?, ?, '', 2)", userID, agentID)
	t.Cleanup(func() {
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM realtime_events WHERE target_type = 'user' AND target_id = ?", fmt.Sprint(userID))
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_provider_attempts WHERE command_id IN (SELECT id FROM ai_reply_commands WHERE conversation_id = ?)", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_reply_commands WHERE conversation_id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_messages WHERE conversation_id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_conversations WHERE id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_agents WHERE id = ?", agentID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	})
	return replyFixture{userID: userID, agentID: agentID, conversationID: conversationID}
}

func assertReplyRows(t *testing.T, db *database.Client, conversationID int64, requestID string, content string, wantCommands int, wantMessages int) {
	t.Helper()
	var commandCount int
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_reply_commands WHERE conversation_id = ? AND request_id = ?", conversationID, requestID).Scan(&commandCount); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if commandCount != wantCommands {
		t.Fatalf("commands for %q = %d, want %d", requestID, commandCount, wantCommands)
	}
	var messageCount int
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_messages WHERE conversation_id = ? AND content = ?", conversationID, content).Scan(&messageCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != wantMessages {
		t.Fatalf("messages for %q = %d, want %d", content, messageCount, wantMessages)
	}
}

func insertCommandKeyBlocker(t *testing.T, db *database.Client, fixture replyFixture, key string) {
	t.Helper()
	fingerprint := sha256.Sum256([]byte("reply-command-idempotency-blocker:" + key))
	_, err := db.SQL.ExecContext(context.Background(), `
INSERT INTO ai_reply_commands
  (request_id, request_fingerprint, request_identity_status, request_identity_marker, idempotency_key, platform, user_id, conversation_id, user_message_id, state, max_attempts, next_attempt_at)
VALUES
	(?, ?, 'replayable', '', ?, 'admin', ?, ?, ?, 'failed', 3, CURRENT_TIMESTAMP(6))`, "blocker", fingerprint[:], key, fixture.userID, fixture.conversationID, -time.Now().UnixNano())
	if err != nil {
		t.Fatalf("reserve command idempotency key: %v", err)
	}
}
