package aimessage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/enum"
)

type fakeRepository struct {
	conversation         *Conversation
	agent                *AgentRuntime
	rows                 []Message
	listQuery            ListQuery
	replyInput           replycommand.CreateReplyInput
	replyResult          replycommand.CreateReplyResult
	replyErr             error
	cancelConversationID int64
	cancelUserID         int64
	cancelRequestID      string
	cancelResult         *replycommand.Command
	cancelErr            error
}

func (f *fakeRepository) RequestCancel(_ context.Context, conversationID int64, userID int64, requestID string, _ time.Time) (*replycommand.Command, error) {
	f.cancelConversationID = conversationID
	f.cancelUserID = userID
	f.cancelRequestID = requestID
	if f.cancelResult == nil {
		f.cancelResult = &replycommand.Command{ID: 99, ConversationID: conversationID, UserID: userID, RequestID: requestID, State: replycommand.StateCanceled}
	}
	return f.cancelResult, f.cancelErr
}

func (f *fakeRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error) {
	return f.agent, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Message, bool, error) {
	f.listQuery = query
	return f.rows, len(f.rows) > query.Limit, nil
}
func (f *fakeRepository) CreateReply(ctx context.Context, input replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error) {
	f.replyInput = input
	if f.replyResult.CommandID == 0 {
		f.replyResult = replycommand.CreateReplyResult{UserMessageID: 12, CommandID: 99, RequestID: input.RequestID, State: replycommand.StatePending}
	}
	return f.replyResult, f.replyErr
}

type fakeCancelPublisher struct {
	commandID uint64
	err       error
}

type fakeReplyWaker struct {
	commandID uint64
	err       error
}

func (f *fakeReplyWaker) WakeReply(_ context.Context, commandID uint64) error {
	f.commandID = commandID
	return f.err
}

func (f *fakeCancelPublisher) PublishCancel(_ context.Context, commandID uint64) error {
	f.commandID = commandID
	return f.err
}

func TestListUsesMessageCursorAndReturnsChronologicalOrder(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []Message{
		{ID: 11, ConversationID: 3, Role: enum.AIMessageRoleAssistant, ContentType: "text", Content: "second", CreatedAt: now, UpdatedAt: now},
		{ID: 10, ConversationID: 3, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "first", CreatedAt: now, UpdatedAt: now},
	}}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3, BeforeID: 20})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.Limit != 20 || repo.listQuery.BeforeID != 20 {
		t.Fatalf("unexpected list query: %#v", repo.listQuery)
	}
	if len(res.List) != 2 || res.List[0].ID != 10 || res.List[1].ID != 11 || res.List[0].ContentType != "text" {
		t.Fatalf("unexpected response: %#v", res)
	}
}

func TestListRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 8}}
	_, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr == nil || appErr.LegacyCode != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestSendCommitsTextUserMessageAndDurableReplyCommand(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["chat"]`},
	}
	res, appErr := NewService(repo).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: " hello ", RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if res.UserMessageID != 12 || res.CommandID != 99 || res.ConversationID != 3 || res.RequestID != "rid" || res.State != replycommand.StatePending {
		t.Fatalf("unexpected response: %#v", res)
	}
	if repo.replyInput.Content != "hello" || repo.replyInput.ConversationID != 3 || repo.replyInput.UserID != 7 || repo.replyInput.RequestID != "rid" {
		t.Fatalf("unexpected durable reply input: %#v", repo.replyInput)
	}
	if repo.replyInput.MetaJSON != nil {
		t.Fatalf("empty metadata must be stored as nil, got %#v", repo.replyInput.MetaJSON)
	}
}

func TestSendWakesCommittedCommandAndDoesNotFailWhenWakeupFails(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["chat"]`},
	}
	waker := &fakeReplyWaker{err: errors.New("redis unavailable")}
	res, appErr := NewService(repo, WithReplyWaker(waker)).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "hello", RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("durable send must survive best-effort wake failure: %v", appErr)
	}
	if res.CommandID != 99 || waker.commandID != 99 {
		t.Fatalf("response=%+v wake command=%d", res, waker.commandID)
	}
}

func TestSendKeepsImageAttachmentsInMetaJSON(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["chat"]`},
	}
	_, appErr := NewService(repo).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "看图", RequestID: "rid", Attachments: []Attachment{{Type: "image", URL: "https://example.test/a.png", Name: "a.png", Size: 10}}})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if repo.replyInput.MetaJSON == nil || !strings.Contains(*repo.replyInput.MetaJSON, "attachments") || !strings.Contains(*repo.replyInput.MetaJSON, "https://example.test/a.png") {
		t.Fatalf("missing attachment meta json: %#v", repo.replyInput.MetaJSON)
	}
}

func TestCancelRequiresOwnedConversation(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}}
	publisher := &fakeCancelPublisher{err: errors.New("redis unavailable")}
	res, appErr := NewService(repo, WithCancelPublisher(publisher)).Cancel(context.Background(), 7, CancelInput{ConversationID: 3, RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("Cancel returned error: %v", appErr)
	}
	if res.ConversationID != 3 || res.RequestID != "rid" || res.Status != "canceled" {
		t.Fatalf("unexpected cancel response: %#v", res)
	}
	if repo.cancelConversationID != 3 || repo.cancelUserID != 7 || repo.cancelRequestID != "rid" || publisher.commandID != 99 {
		t.Fatalf("durable cancel repo=(%d,%d,%q) signal=%d", repo.cancelConversationID, repo.cancelUserID, repo.cancelRequestID, publisher.commandID)
	}
}

func TestSendRejectsNonChatAgent(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["image"]`}}
	_, appErr := NewService(repo).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "hello", RequestID: "rid"})
	if appErr == nil || appErr.LegacyCode != 100 || appErr.Message != "该智能体不支持对话场景" {
		t.Fatalf("expected non-chat bad request, got %#v", appErr)
	}
}
