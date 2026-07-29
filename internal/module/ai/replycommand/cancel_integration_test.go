package replycommand

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/redisclient"
)

func TestRedisCancelSignalCrossesProcessBoundary(t *testing.T) {
	if os.Getenv(durableWorkIntegrationEnv) != "1" {
		t.Skip("Docker-only durable work integration test")
	}
	client, err := redisclient.Open(config.RedisConfig{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	commandID := uint64(time.Now().UnixNano())
	subscription, err := NewRedisCancelSubscriber(client).SubscribeCancel(context.Background(), commandID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscription.Close() })
	if err := NewRedisCancelPublisher(client).PublishCancel(context.Background(), commandID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscription.Signal():
	case <-time.After(2 * time.Second):
		t.Fatal("cancel signal was not delivered")
	}
}

func TestRequestCancelIsDurableAndIdempotentForPendingAndRunningCommands(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }

	pending, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "cancel-pending", "pending"))
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := repository.RequestCancel(ctx, fixture.conversationID, fixture.userID, "cancel-pending", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if canceled.ID != pending.CommandID || canceled.State != StatePending || canceled.CancelRequestedAt == nil || canceled.FinishedAt != nil {
		t.Fatalf("pending cancel=%+v", canceled)
	}
	firstRequestedAt := *canceled.CancelRequestedAt
	again, err := repository.RequestCancel(ctx, fixture.conversationID, fixture.userID, "cancel-pending", now.Add(2*time.Second))
	if err != nil || again.ID != canceled.ID || again.State != StatePending || again.CancelRequestedAt == nil || !again.CancelRequestedAt.Equal(firstRequestedAt) {
		t.Fatalf("idempotent cancel=%+v err=%v", again, err)
	}

	running, err := repository.CreateReply(ctx, testCreateReplyInput(fixture.conversationID, fixture.userID, "cancel-running", "running"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimByID(ctx, running.CommandID, ClaimSourcePoll, "worker-a", now, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if ok, err := repository.Transition(ctx, running.CommandID, "worker-a", claim.FencingToken, StateClaimed, StateRunning, nil); err != nil || !ok {
		t.Fatalf("running transition ok=%v err=%v", ok, err)
	}
	requested, err := repository.RequestCancel(ctx, fixture.conversationID, fixture.userID, "cancel-running", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if requested.State != StateRunning || requested.CancelRequestedAt == nil || requested.FinishedAt != nil {
		t.Fatalf("running cancel request=%+v", requested)
	}
	if assistantID, published, err := repository.PublishAssistant(ctx, PublishAssistantInput{CommandID: running.CommandID, Owner: "worker-a", Token: claim.FencingToken, Content: "must not publish", Now: now.Add(2 * time.Second)}); err != nil || published || assistantID != 0 {
		t.Fatalf("canceled command publication id=%d published=%v err=%v", assistantID, published, err)
	}

	if _, err := repository.RequestCancel(ctx, fixture.conversationID, fixture.userID+1, "cancel-running", now); !errors.Is(err, ErrConversationUnavailable) {
		t.Fatalf("unauthorized cancel err=%v", err)
	}
	if _, err := repository.RequestCancel(ctx, fixture.conversationID, fixture.userID, strings.Repeat("界", 129), now); !errors.Is(err, ErrCreateInputInvalid) {
		t.Fatalf("oversized cancel request_id err=%v", err)
	}
}
