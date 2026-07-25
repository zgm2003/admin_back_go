package replycommand

import (
	"context"
	"testing"
	"time"

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
	claim, err := repository.ClaimByID(ctx, created.CommandID, "worker-a", now, time.Minute)
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
	claim, err := repository.ClaimByID(context.Background(), created.CommandID, "worker", now, time.Minute)
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
	claim, err := repository.ClaimByID(ctx, created.CommandID, "worker-a", now, time.Second)
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
	if reclaimed, err := repository.ClaimByID(ctx, created.CommandID, "worker-b", now.Add(2*time.Second), time.Second); err != nil || reclaimed != nil {
		t.Fatalf("dispatched request was blindly reclaimed: claim=%+v err=%v", reclaimed, err)
	}
	var state string
	if err := db.SQL.QueryRowContext(ctx, "SELECT state FROM ai_reply_commands WHERE id = ?", created.CommandID).Scan(&state); err != nil || state != string(StateOutcomeUnknown) {
		t.Fatalf("state=%q err=%v", state, err)
	}
}
