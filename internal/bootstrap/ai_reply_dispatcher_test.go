package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/aimessage"
)

type fakeConversationReplyService struct {
	done    chan aichat.ConversationReplyInput
	blocked chan struct{}
}

func (f *fakeConversationReplyService) ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error) {
	if f.blocked != nil {
		<-ctx.Done()
		f.done <- input
		return nil, ctx.Err()
	}
	f.done <- input
	return &aichat.ConversationReplyResult{ConversationID: input.ConversationID, AssistantMessageID: 22}, nil
}

func TestAIConversationReplyDispatcherExecutesReplyInAPIProcess(t *testing.T) {
	service := &fakeConversationReplyService{done: make(chan aichat.ConversationReplyInput, 1)}
	dispatcher := newAIConversationReplyDispatcher(service, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Second)
	defer func() {
		if err := dispatcher.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	}()

	err := dispatcher.EnqueueConversationReply(context.Background(), aimessage.ReplyPayload{
		ConversationID: 3,
		UserID:         7,
		AgentID:        5,
		UserMessageID:  9,
		RequestID:      "rid",
	})
	if err != nil {
		t.Fatalf("EnqueueConversationReply returned error: %v", err)
	}

	select {
	case got := <-service.done:
		if got.ConversationID != 3 || got.UserID != 7 || got.AgentID != 5 || got.UserMessageID != 9 || got.RequestID != "rid" {
			t.Fatalf("unexpected input: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("reply was not dispatched")
	}
}

func TestAIConversationReplyDispatcherCancelsRunningReplyByConversationAndRequest(t *testing.T) {
	service := &fakeConversationReplyService{
		done:    make(chan aichat.ConversationReplyInput, 1),
		blocked: make(chan struct{}),
	}
	dispatcher := newAIConversationReplyDispatcher(service, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Second)
	defer func() {
		if err := dispatcher.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	}()

	payload := aimessage.ReplyPayload{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"}
	if err := dispatcher.EnqueueConversationReply(context.Background(), payload); err != nil {
		t.Fatalf("EnqueueConversationReply returned error: %v", err)
	}
	if err := dispatcher.CancelConversationReply(context.Background(), payload); err != nil {
		t.Fatalf("CancelConversationReply returned error: %v", err)
	}

	select {
	case got := <-service.done:
		if got.ConversationID != 3 || got.RequestID != "rid" {
			t.Fatalf("unexpected canceled input: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("reply was not canceled")
	}
}
