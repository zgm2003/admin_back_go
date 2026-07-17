package replycommand

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	aichat "admin_back_go/internal/module/ai/chat"
)

type publishingReplyExecutor struct {
	repository *GormRepository
}

func (e publishingReplyExecutor) ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	id, published, err := e.repository.PublishAssistant(ctx, PublishAssistantInput{CommandID: input.CommandID, Owner: input.LeaseOwner, Token: input.LeaseToken, Content: "restart answer", Now: time.Now()})
	if err != nil || !published {
		return nil, err
	}
	return &aichat.ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: id}, nil
}

func TestRunnerRestartPollsCommittedCommandAndDuplicateWakeIsNoop(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	created, err := repository.CreateReply(context.Background(), CreateReplyInput{ConversationID: fixture.conversationID, UserID: fixture.userID, RequestID: "restart-request", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}

	restartedWorker := NewRunner(RunnerOptions{Repository: repository, Executor: publishingReplyExecutor{repository: repository}, Owner: "worker-after-api-exit", LeaseTTL: time.Minute})
	worked, err := restartedWorker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("restart poll worked=%v err=%v", worked, err)
	}
	worked, err = restartedWorker.RunCommand(context.Background(), created.CommandID)
	if err != nil || worked {
		t.Fatalf("duplicate wake worked=%v err=%v", worked, err)
	}
	var assistantID int64
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT assistant_message_id FROM ai_reply_commands WHERE id = ? AND state = ?", created.CommandID, StateSucceeded).Scan(&assistantID); err != nil || assistantID == 0 {
		t.Fatalf("assistant id=%d err=%v", assistantID, err)
	}
}

func TestClaimLeaseFencingAndIdempotentAssistantPublication(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }
	created, err := repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      "lease-request",
		Content:        "hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	claimA, err := repository.ClaimNext(ctx, "worker-a", now, 3*time.Second)
	if err != nil || claimA == nil {
		t.Fatalf("worker A claim=%+v err=%v", claimA, err)
	}
	if claimA.Command.ID != created.CommandID || claimA.FencingToken != 1 {
		t.Fatalf("unexpected worker A claim: %+v", claimA)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, "worker-a", claimA.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("worker A running transition ok=%v err=%v", ok, err)
	}

	claimB, err := repository.ClaimNext(ctx, "worker-b", now.Add(4*time.Second), 3*time.Second)
	if err != nil || claimB == nil {
		t.Fatalf("worker B reclaim=%+v err=%v", claimB, err)
	}
	if claimB.Command.ID != created.CommandID || claimB.FencingToken != 2 {
		t.Fatalf("unexpected worker B claim: %+v", claimB)
	}
	if renewed, err := repository.Renew(ctx, created.CommandID, "worker-a", claimA.FencingToken, now.Add(4*time.Second)); err != nil || renewed.Alive {
		t.Fatalf("stale worker renewed=%+v err=%v", renewed, err)
	}
	if ok, err := repository.Transition(ctx, created.CommandID, "worker-a", claimA.FencingToken, StateClaimed, StateRunning, nil); err != nil || ok {
		t.Fatalf("stale worker transitioned ok=%v err=%v", ok, err)
	}
	if _, ok, err := repository.PublishAssistant(ctx, PublishAssistantInput{
		CommandID: created.CommandID,
		Owner:     "worker-a",
		Token:     claimA.FencingToken,
		Content:   "stale answer",
		Now:       now.Add(4 * time.Second),
	}); err != nil || ok {
		t.Fatalf("stale publication ok=%v err=%v", ok, err)
	}

	if ok, err := repository.Transition(ctx, created.CommandID, "worker-b", claimB.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("worker B running transition ok=%v err=%v", ok, err)
	}
	assistantID, ok, err := repository.PublishAssistant(ctx, PublishAssistantInput{
		CommandID: created.CommandID,
		Owner:     "worker-b",
		Token:     claimB.FencingToken,
		Content:   "durable answer",
		Now:       now.Add(5 * time.Second),
	})
	if err != nil || !ok || assistantID == 0 {
		t.Fatalf("worker B publication id=%d ok=%v err=%v", assistantID, ok, err)
	}
	duplicateID, ok, err := repository.PublishAssistant(ctx, PublishAssistantInput{
		CommandID: created.CommandID,
		Owner:     "worker-b",
		Token:     claimB.FencingToken,
		Content:   "duplicate answer",
		Now:       now.Add(6 * time.Second),
	})
	if err != nil || !ok || duplicateID != assistantID {
		t.Fatalf("duplicate publication id=%d ok=%v err=%v", duplicateID, ok, err)
	}
	if staleID, staleOK, err := repository.PublishAssistant(ctx, PublishAssistantInput{
		CommandID: created.CommandID,
		Owner:     "worker-a",
		Token:     claimA.FencingToken,
		Content:   "stale duplicate",
		Now:       now.Add(7 * time.Second),
	}); err != nil || staleOK || staleID != 0 {
		t.Fatalf("stale duplicate publication id=%d ok=%v err=%v", staleID, staleOK, err)
	}
	assertPublishedCommand(t, db, created.CommandID, assistantID)
}

func assertPublishedCommand(t *testing.T, db *database.Client, commandID uint64, assistantID int64) {
	t.Helper()
	var state string
	var linkedID int64
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT state, assistant_message_id FROM ai_reply_commands WHERE id = ?", commandID).Scan(&state, &linkedID); err != nil {
		t.Fatalf("query published command: %v", err)
	}
	if state != string(StateSucceeded) || linkedID != assistantID {
		t.Fatalf("command state=%q assistant=%d", state, linkedID)
	}
	var count int
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_messages WHERE reply_command_id = ? AND role = 2", commandID).Scan(&count); err != nil {
		t.Fatalf("count assistant messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("assistant message count=%d, want 1", count)
	}
}
