package replycommand

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"
)

func TestOutcomeUnknownReconciliationUsesLocalTruthOrFailsWithoutResend(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	eventRepository := modulerealtime.NewGormRepository(db, modulerealtime.DefaultRegistry())
	repository := NewGormRepository(db, WithDurableEventSink(modulerealtime.NewDurableEventSink(eventRepository, infrarealtime.NoopPublisher{}, slog.Default())))
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }

	createUnknown := func(requestID string) CreateReplyResult {
		created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, requestID, requestID))
		if err != nil {
			t.Fatal(err)
		}
		claim, err := repository.ClaimByID(ctx, created.CommandID, "worker-a", now, time.Minute)
		if err != nil || claim == nil {
			t.Fatalf("claim=%+v err=%v", claim, err)
		}
		if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
			t.Fatalf("running ok=%v err=%v", ok, err)
		}
		if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateOutcomeUnknown, map[string]any{"outcome_unknown_at": now}); err != nil || !ok {
			t.Fatalf("unknown ok=%v err=%v", ok, err)
		}
		return created
	}

	withLocal := createUnknown("unknown-local")
	result, err := db.SQL.ExecContext(ctx, "INSERT INTO ai_messages (conversation_id, reply_command_id, role, content_type, content, is_del, created_at, updated_at) VALUES (?, ?, ?, 'text', 'local answer', ?, ?, ?)", fixture.conversationID, withLocal.CommandID, enum.AIMessageRoleAssistant, enum.CommonNo, now, now)
	if err != nil {
		t.Fatal(err)
	}
	localAssistantID, _ := result.LastInsertId()
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository, Now: func() time.Time { return now.Add(time.Second) }})
	if worked, err := reconciler.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("local reconcile worked=%v err=%v", worked, err)
	}
	assertReconciledCommand(t, db, withLocal.CommandID, StateSucceeded, localAssistantID)

	withoutLookup := createUnknown("unknown-unqueryable")
	if worked, err := reconciler.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("unknown reconcile worked=%v err=%v", worked, err)
	}
	assertReconciledCommand(t, db, withoutLookup.CommandID, StateFailed, 0)
	resumed, err := eventRepository.ResumeUser(ctx, modulerealtime.ResumeQuery{UserID: fixture.userID, AfterSequence: 0, Now: now.Add(time.Second)})
	if err != nil || len(resumed.Events) != 2 {
		t.Fatalf("reconciled durable events=%#v err=%v", resumed, err)
	}
	if resumed.Events[0].EventType != modulerealtime.TypeAIResponseCompletedV1 || resumed.Events[1].EventType != modulerealtime.TypeAIResponseFailedV1 {
		t.Fatalf("unexpected reconciled event types: %#v", resumed.Events)
	}
}

func assertReconciledCommand(t *testing.T, db *database.Client, commandID uint64, wantState State, wantAssistantID int64) {
	t.Helper()
	var state string
	var assistantID *int64
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT state, assistant_message_id FROM ai_reply_commands WHERE id = ?", commandID).Scan(&state, &assistantID); err != nil {
		t.Fatal(err)
	}
	if state != string(wantState) || (wantAssistantID > 0 && (assistantID == nil || *assistantID != wantAssistantID)) || (wantAssistantID == 0 && assistantID != nil) {
		t.Fatalf("state=%q assistant=%v want state=%q assistant=%d", state, assistantID, wantState, wantAssistantID)
	}
}
