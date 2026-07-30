package replycommand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
	modulerealtime "admin_back_go/internal/module/realtime"

	"gorm.io/gorm"
)

func TestResumeAICompletionIsAtomicAndIdempotent(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	eventRepository := modulerealtime.NewGormRepository(db, modulerealtime.DefaultRegistry())
	eventSink := modulerealtime.NewDurableEventSink(eventRepository, infrarealtime.NoopPublisher{}, slog.Default())
	repository := NewGormRepository(db, WithDurableEventSink(eventSink))
	ctx := context.Background()
	now := time.Now()
	requestID := fmt.Sprintf("resume-completed-%d", now.UnixNano())
	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, requestID, "hello"))
	if err != nil {
		t.Fatalf("CreateReply: %v", err)
	}
	claimAt := time.Now().Add(time.Second)
	claim, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker-a", claimAt, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, map[string]any{"started_at": claimAt}); err != nil || !ok {
		t.Fatalf("running transition ok=%v err=%v", ok, err)
	}
	assistantID, published, err := repository.PublishAssistant(ctx, PublishAssistantInput{CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Content: "answer", Now: claimAt.Add(time.Second)})
	if err != nil || !published || assistantID <= 0 {
		t.Fatalf("PublishAssistant id=%d published=%v err=%v", assistantID, published, err)
	}
	duplicateID, duplicatePublished, err := repository.PublishAssistant(ctx, PublishAssistantInput{CommandID: created.CommandID, Owner: claim.Owner, Token: claim.FencingToken, Content: "different", Now: claimAt.Add(2 * time.Second)})
	if err != nil || !duplicatePublished || duplicateID != assistantID {
		t.Fatalf("duplicate publication id=%d published=%v err=%v", duplicateID, duplicatePublished, err)
	}

	var count int64
	if err := db.Gorm.Table("realtime_events").
		Where("target_type = ? AND target_id = ? AND event_type = ? AND request_id = ?", modulerealtime.TargetTypeUser, fmt.Sprint(fixture.userID), modulerealtime.TypeAIResponseCompletedV1, requestID).
		Count(&count).Error; err != nil {
		t.Fatalf("count completed realtime events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one completed realtime event, got %d", count)
	}
	resumed, err := eventRepository.ResumeUser(ctx, modulerealtime.ResumeQuery{UserID: fixture.userID, AfterSequence: 0, Now: claimAt.Add(time.Second)})
	if err != nil || len(resumed.Events) != 1 || resumed.Events[0].EventType != modulerealtime.TypeAIResponseCompletedV1 {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
}

func TestResumeAIFailureIsCommittedWithTerminalTransition(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	eventRepository := modulerealtime.NewGormRepository(db, modulerealtime.DefaultRegistry())
	eventSink := modulerealtime.NewDurableEventSink(eventRepository, infrarealtime.NoopPublisher{}, slog.Default())
	repository := NewGormRepository(db, WithDurableEventSink(eventSink))
	ctx := context.Background()
	requestID := fmt.Sprintf("resume-failed-%d", time.Now().UnixNano())
	created, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, requestID, "hello"))
	if err != nil {
		t.Fatalf("CreateReply: %v", err)
	}
	claimAt := time.Now().Add(time.Second)
	claim, err := repository.ClaimByID(ctx, created.CommandID, ClaimSourcePoll, "worker-a", claimAt, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, map[string]any{"started_at": claimAt}); err != nil || !ok {
		t.Fatalf("running transition ok=%v err=%v", ok, err)
	}
	finishedAt := claimAt.Add(time.Second)
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateFailed, map[string]any{
		"last_error_code": "ai.reply_failed", "last_error_message": "provider failed",
	}); err == nil || ok {
		t.Fatalf("failure without an explicit finished_at was accepted ok=%v err=%v", ok, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateFailed, map[string]any{
		"finished_at": finishedAt, "last_error_code": "ai.reply_failed", "last_error_message": "",
	}); err == nil || ok {
		t.Fatalf("failure without an explicit message was accepted ok=%v err=%v", ok, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateFailed, map[string]any{
		"finished_at": finishedAt, "last_error_code": "ai.reply_failed", "last_error_message": "provider failed",
	}); err != nil || !ok {
		t.Fatalf("failed transition ok=%v err=%v", ok, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateFailed, map[string]any{
		"finished_at": finishedAt, "last_error_message": "duplicate",
	}); err != nil || ok {
		t.Fatalf("duplicate terminal transition ok=%v err=%v", ok, err)
	}

	resumed, err := eventRepository.ResumeUser(ctx, modulerealtime.ResumeQuery{UserID: fixture.userID, AfterSequence: 0, Now: finishedAt})
	if err != nil || len(resumed.Events) != 1 {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	envelope, err := resumed.Events[0].Envelope(modulerealtime.DefaultRegistry())
	if err != nil {
		t.Fatalf("failed envelope: %v", err)
	}
	if envelope.Type != modulerealtime.TypeAIResponseFailedV1 {
		t.Fatalf("unexpected terminal event: %#v", envelope)
	}
	var payload modulerealtime.AIResponseFailedPayload
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode failed payload: %v", err)
	}
	if payload.ConversationID != fixture.conversationID || payload.RequestID != requestID || payload.Msg != "provider failed" {
		t.Fatalf("unexpected failed payload: %#v", payload)
	}
}

func TestResumeAICancellationIsCommittedExactlyOnceForPendingAndRunning(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	eventRepository := modulerealtime.NewGormRepository(db, modulerealtime.DefaultRegistry())
	eventSink := modulerealtime.NewDurableEventSink(eventRepository, infrarealtime.NoopPublisher{}, slog.Default())
	repository := NewGormRepository(db, WithDurableEventSink(eventSink))
	ctx := context.Background()
	now := time.Now().UTC()

	pendingRequestID := fmt.Sprintf("resume-canceled-pending-%d", now.UnixNano())
	pending, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, pendingRequestID, "pending"))
	if err != nil {
		t.Fatalf("create pending reply: %v", err)
	}
	if _, err := repository.RequestCancel(ctx, requestCancelInput(fixture.conversationID, fixture.userID, pendingRequestID, 0, now)); err != nil {
		t.Fatalf("cancel pending reply: %v", err)
	}
	if _, err := repository.RequestCancel(ctx, requestCancelInput(fixture.conversationID, fixture.userID, pendingRequestID, 0, now.Add(time.Second))); err != nil {
		t.Fatalf("repeat pending cancel: %v", err)
	}
	claimPending, err := repository.ClaimByID(ctx, pending.CommandID, ClaimSourcePoll, "worker-pending", now.Add(2*time.Second), time.Minute)
	if err != nil || claimPending == nil {
		t.Fatalf("claim canceled pending reply=%#v err=%v", claimPending, err)
	}
	if ok, err := repository.Transition(ctx, pending.CommandID, claimPending.Owner, claimPending.FencingToken, StateClaimed, StateRunning, map[string]any{"started_at": now.Add(2 * time.Second)}); err != nil || !ok {
		t.Fatalf("start canceled pending reply ok=%v err=%v", ok, err)
	}
	if ok, err := repository.Transition(ctx, pending.CommandID, claimPending.Owner, claimPending.FencingToken, StateRunning, StateCanceled, map[string]any{"finished_at": now.Add(3 * time.Second)}); err != nil || !ok {
		t.Fatalf("finish canceled pending reply ok=%v err=%v", ok, err)
	}
	assertSingleCanceledEvent(t, db.Gorm, eventRepository, fixture.userID, fixture.conversationID, pendingRequestID)

	runningRequestID := fmt.Sprintf("resume-canceled-running-%d", now.UnixNano())
	running, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, runningRequestID, "running"))
	if err != nil {
		t.Fatalf("create running reply: %v", err)
	}
	claimAt := now.Add(time.Minute)
	claim, err := repository.ClaimByID(ctx, running.CommandID, ClaimSourcePoll, "worker-a", claimAt, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim running reply=%#v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, running.CommandID, claim.Owner, claim.FencingToken, StateClaimed, StateRunning, map[string]any{"started_at": claimAt}); err != nil || !ok {
		t.Fatalf("start running reply ok=%v err=%v", ok, err)
	}
	if _, err := repository.RequestCancel(ctx, requestCancelInput(fixture.conversationID, fixture.userID, runningRequestID, 0, claimAt.Add(time.Second))); err != nil {
		t.Fatalf("request running cancel: %v", err)
	}
	if ok, err := repository.Transition(ctx, running.CommandID, claim.Owner, claim.FencingToken, StateRunning, StateCanceled, map[string]any{"finished_at": claimAt.Add(2 * time.Second)}); err != nil || !ok {
		t.Fatalf("finish running cancel ok=%v err=%v", ok, err)
	}
	assertSingleCanceledEvent(t, db.Gorm, eventRepository, fixture.userID, fixture.conversationID, runningRequestID)

	var state State
	if err := db.Gorm.Model(&Command{}).Select("state").Where("id = ?", pending.CommandID).Scan(&state).Error; err != nil || state != StateCanceled {
		t.Fatalf("pending state=%q err=%v", state, err)
	}
}

type failingCanceledEventSink struct{}

func (failingCanceledEventSink) AppendTx(context.Context, *gorm.DB, modulerealtime.AppendInput) (*modulerealtime.Event, error) {
	return nil, errors.New("event insert failed")
}

func (failingCanceledEventSink) PublishBestEffort(context.Context, *modulerealtime.Event) {}

func TestCancelIntentDoesNotDependOnTerminalEventSink(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db, WithDurableEventSink(failingCanceledEventSink{}))
	requestID := fmt.Sprintf("cancel-rollback-%d", time.Now().UnixNano())
	created, err := repository.CreateReply(context.Background(), testCreateReplyInput(fixture.conversationID, fixture.userID, requestID, "pending"))
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if _, err := repository.RequestCancel(context.Background(), requestCancelInput(fixture.conversationID, fixture.userID, requestID, 0, time.Now())); err != nil {
		t.Fatalf("persist cancel intent: %v", err)
	}
	var command Command
	if err := db.Gorm.First(&command, "id = ?", created.CommandID).Error; err != nil {
		t.Fatalf("reload command: %v", err)
	}
	if command.State != StatePending || command.CancelRequestedAt == nil || command.FinishedAt != nil {
		t.Fatalf("cancel intent was not persisted independently: %#v", command)
	}
}

func assertSingleCanceledEvent(t *testing.T, db *gorm.DB, events *modulerealtime.GormRepository, userID, conversationID int64, requestID string) {
	t.Helper()
	var rows []modulerealtime.Event
	if err := db.Where("target_type = ? AND target_id = ? AND event_type = ? AND request_id = ?", modulerealtime.TargetTypeUser, fmt.Sprint(userID), modulerealtime.TypeAIResponseCanceledV2, requestID).Find(&rows).Error; err != nil {
		t.Fatalf("load canceled events: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("canceled event count=%d rows=%#v", len(rows), rows)
	}
	envelope, err := rows[0].Envelope(modulerealtime.DefaultRegistry())
	if err != nil {
		t.Fatalf("build canceled envelope: %v", err)
	}
	var payload modulerealtime.AIResponseCanceledPayload
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode canceled payload: %v", err)
	}
	if payload.ConversationID != conversationID || payload.RequestID != requestID || payload.AssistantMessageID <= 0 {
		t.Fatalf("unexpected canceled payload: %#v", payload)
	}
	resumed, err := events.ResumeUser(context.Background(), modulerealtime.ResumeQuery{UserID: userID, AfterSequence: rows[0].Sequence - 1})
	if err != nil || len(resumed.Events) != 1 || resumed.Events[0].EventID != rows[0].EventID {
		t.Fatalf("resume canceled event=%#v err=%v", resumed, err)
	}
}
