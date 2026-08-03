package replycommand

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/aigateway"

	"gorm.io/gorm"
)

func TestProviderAttemptPersistsBeforeDispatchAndRequiresCurrentFence(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }
	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "attempt-request", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker-a", now, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("running transition ok=%v err=%v", ok, err)
	}

	if attempt, ok, err := repository.PrepareLegacyAttempt(ctx, LegacyPrepareAttemptInput{CommandID: created.CommandID, Owner: "stale-worker", Token: claim.FencingToken, Now: now}); err != nil || ok || attempt != nil {
		t.Fatalf("stale attempt=%+v ok=%v err=%v", attempt, ok, err)
	}
	first, ok, err := repository.PrepareLegacyAttempt(ctx, LegacyPrepareAttemptInput{CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Now: now})
	if err != nil || !ok || first == nil || first.ID == 0 || first.AttemptNo != 1 || first.State != AttemptPrepared || len(first.IdempotencyKey) != 64 {
		t.Fatalf("first attempt=%+v ok=%v err=%v", first, ok, err)
	}
	second, ok, err := repository.PrepareLegacyAttempt(ctx, LegacyPrepareAttemptInput{CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Now: now.Add(time.Second)})
	if err != nil || !ok || second == nil || second.AttemptNo != 2 || second.IdempotencyKey == first.IdempotencyKey {
		t.Fatalf("second attempt=%+v ok=%v err=%v", second, ok, err)
	}
	if ok, err := repository.MarkAttemptDispatched(ctx, second.ID, created.CommandID, claim.Owner, claim.FencingToken, now.Add(2*time.Minute)); err != nil || ok {
		t.Fatalf("expired lease dispatched second attempt ok=%v err=%v", ok, err)
	}
	if ok, err := repository.MarkAttemptDispatched(ctx, first.ID, created.CommandID, claim.Owner, claim.FencingToken, now.Add(2*time.Second)); err != nil || !ok {
		t.Fatalf("mark dispatched ok=%v err=%v", ok, err)
	}
	if ok, err := repository.FinishAttempt(ctx, FinishAttemptInput{AttemptID: first.ID, CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, State: AttemptOutcomeUnknown, ProviderRequestID: "provider-request-1", ErrorCode: "ai.provider_outcome_unknown", Now: now.Add(3 * time.Second)}); err != nil || !ok {
		t.Fatalf("finish attempt ok=%v err=%v", ok, err)
	}

	var state string
	var requestID string
	if err := db.SQL.QueryRowContext(ctx, "SELECT state, provider_request_id FROM ai_provider_attempts WHERE id = ?", first.ID).Scan(&state, &requestID); err != nil || state != string(AttemptOutcomeUnknown) || requestID != "provider-request-1" {
		t.Fatalf("persisted state=%q request_id=%q err=%v", state, requestID, err)
	}
}

func TestPreparePaidAttemptRejectsLegacyFallback(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	now := time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC)
	created, err := repository.CreateReply(context.Background(), testCreateReplyInput(fixture.conversationID, fixture.userID, "paid-attempt", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimByID(context.Background(), created.CommandID, ClaimSourcePoll, "worker", now, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if ok, err := repository.Transition(context.Background(), created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("running ok=%v err=%v", ok, err)
	}
	err = db.Gorm.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		attempt, ok, prepareErr := repository.PreparePaidAttemptInTransaction(context.Background(), tx, PrepareAttemptInput{RunID: 99, CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Now: now})
		if prepareErr == nil || ok || attempt != nil {
			t.Fatalf("paid attempt accepted legacy fallback: attempt=%+v ok=%v err=%v", attempt, ok, prepareErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreparePaidAttemptPersistsAndRecoversContextPlanEvidence(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	requestID := "context-plan-attempt"
	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, requestID, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker", now, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("running ok=%v err=%v", ok, err)
	}

	fingerprint := sha256.Sum256([]byte("context-plan-input"))
	runResult, err := db.SQL.ExecContext(ctx, `
INSERT INTO ai_runs
  (platform, conversation_id, request_id, request_fingerprint, request_identity_status,
   request_identity_marker, user_id, agent_id, provider_id, model_id, model_display_name,
   input_snapshot, pricing_snapshot_json, status, billing_status, billing_reason, started_at)
VALUES
  ('admin', ?, ?, ?, 'replayable', '', ?, ?, 0, 'gpt-test', 'GPT Test',
   '{}', '{}', 'running', 'pending', 'pending', ?)`,
		fixture.conversationID, requestID, fingerprint[:], fixture.userID, fixture.agentID, now)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := runResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	planHash := sha256.Sum256([]byte("ready context plan"))
	capabilityHash := sha256.Sum256([]byte("model capability"))
	planResult, err := db.SQL.ExecContext(ctx, `
INSERT INTO ai_context_plans
  (run_id, policy_version, input_fingerprint_sha256, plan_sha256, model_capability_sha256,
   api_protocol_snapshot, token_counter_id_snapshot, context_window_tokens,
   effective_output_tokens, provider_protocol_upper_bound, tool_continuation_input_reserve,
   policy_safety_margin, known_input_budget, known_input_upper_bound, budget_proof,
   retrieval_outcome, state, metrics_json)
VALUES
  (?, 'test-policy-v1', ?, ?, ?, 'responses', 'unicode_codepoint_v1', 100, 10, 10, 0,
   10, 70, 10, 'exact', 'skipped', 'ready', '{"schema":"context_plan_metrics_v1"}')`,
		runID, fingerprint[:], planHash[:], capabilityHash[:])
	if err != nil {
		t.Fatal(err)
	}
	planID, err := planResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_provider_attempts WHERE run_id = ?", runID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_context_plans WHERE id = ?", planID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_runs WHERE id = ?", runID)
	})

	request := `{"model":"gpt-test"}`
	requestHash := sha256.Sum256([]byte(request))
	evidence := &aigateway.ContextPlanEvidence{ID: uint64(planID), SHA256: planHash}
	input := PrepareAttemptInput{
		RunID: runID, CommandID: created.CommandID, AttemptNo: 1,
		Owner: claim.Owner, Token: claim.FencingToken, Now: now, PrepareStartedAt: now,
		IdempotencyKey: providerAttemptKey(uint64(runID), 1), PreparedRequestJSON: request,
		PreparedRequestSHA256: requestHash, QuoteJSON: `{"pricing_version":"v1","target_hold_units":1}`,
		ContextPlan: evidence,
	}
	var prepared *Attempt
	err = db.Gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, ok, prepareErr := repository.PreparePaidAttemptInTransaction(ctx, tx, input)
		if prepareErr != nil {
			return prepareErr
		}
		if !ok || row == nil {
			t.Fatal("prepared attempt was not persisted")
		}
		prepared = row
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := repository.GetPreparedAttempt(ctx, runID, 1)
	if err != nil {
		t.Fatal(err)
	}
	recoveredEvidence, err := ContextPlanEvidenceFromAttempt(*recovered)
	if err != nil {
		t.Fatal(err)
	}
	if prepared == nil || recoveredEvidence == nil || recoveredEvidence.ID != evidence.ID || recoveredEvidence.SHA256 != evidence.SHA256 {
		t.Fatalf("prepared=%+v recovered=%+v", prepared, recoveredEvidence)
	}
}

func TestExpiredDispatchedAttemptBecomesOutcomeUnknownInsteadOfBlindRetry(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 11, 30, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }
	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "dispatched-recovery", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker-a", now, time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("running ok=%v err=%v", ok, err)
	}
	attempt, ok, err := repository.PrepareLegacyAttempt(ctx, LegacyPrepareAttemptInput{CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Now: now})
	if err != nil || !ok {
		t.Fatalf("attempt=%+v ok=%v err=%v", attempt, ok, err)
	}
	if ok, err := repository.MarkAttemptDispatched(ctx, attempt.ID, created.CommandID, claim.Owner, claim.FencingToken, now); err != nil || !ok {
		t.Fatalf("dispatched ok=%v err=%v", ok, err)
	}
	if reclaimed, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker-b", now.Add(2*time.Second), time.Second); err != nil || reclaimed != nil {
		t.Fatalf("dispatched request was blindly reclaimed: claim=%+v err=%v", reclaimed, err)
	}
	var state string
	if err := db.SQL.QueryRowContext(ctx, "SELECT state FROM ai_reply_commands WHERE id = ?", created.CommandID).Scan(&state); err != nil || state != string(StateOutcomeUnknown) {
		t.Fatalf("state=%q err=%v", state, err)
	}
}
