package aimessage

import (
	"context"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	conversation *Conversation
	agent        *AgentRuntime
	rows         []Message
	listQuery    ListQuery
	sendInput    SendRecord
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
func (f *fakeRepository) InsertUserMessage(ctx context.Context, input SendRecord) (int64, error) {
	f.sendInput = input
	return 12, nil
}

type fakeReplyEnqueuer struct {
	payload       ReplyPayload
	cancelPayload ReplyPayload
}

func (f *fakeReplyEnqueuer) EnqueueConversationReply(ctx context.Context, payload ReplyPayload) error {
	f.payload = payload
	return nil
}

func (f *fakeReplyEnqueuer) CancelConversationReply(ctx context.Context, payload ReplyPayload) error {
	f.cancelPayload = payload
	return nil
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
	if appErr == nil || appErr.Code != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestSendCreatesTextUserMessageAndEnqueuesConversationReply(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["chat"]`},
	}
	enq := &fakeReplyEnqueuer{}
	res, appErr := NewService(repo, WithReplyEnqueuer(enq)).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: " hello ", RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if res.UserMessageID != 12 || res.ConversationID != 3 || res.RequestID != "rid" {
		t.Fatalf("unexpected response: %#v", res)
	}
	if repo.sendInput.Content != "hello" || repo.sendInput.ContentType != "text" || repo.sendInput.Role != enum.AIMessageRoleUser {
		t.Fatalf("unexpected send input: %#v", repo.sendInput)
	}
	if repo.sendInput.MetaJSON != nil {
		t.Fatalf("empty metadata must be stored as nil, got %#v", repo.sendInput.MetaJSON)
	}
	if enq.payload.ConversationID != 3 || enq.payload.UserID != 7 || enq.payload.AgentID != 5 || enq.payload.UserMessageID != 12 || enq.payload.RequestID != "rid" {
		t.Fatalf("unexpected reply payload: %#v", enq.payload)
	}
}

func TestSendKeepsImageAttachmentsInMetaJSON(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["chat"]`},
	}
	_, appErr := NewService(repo, WithReplyEnqueuer(&fakeReplyEnqueuer{})).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "看图", RequestID: "rid", Attachments: []Attachment{{Type: "image", URL: "https://example.test/a.png", Name: "a.png", Size: 10}}})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if repo.sendInput.MetaJSON == nil || !strings.Contains(*repo.sendInput.MetaJSON, "attachments") || !strings.Contains(*repo.sendInput.MetaJSON, "https://example.test/a.png") {
		t.Fatalf("missing attachment meta json: %#v", repo.sendInput.MetaJSON)
	}
}

func TestCancelRequiresOwnedConversation(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}}
	enq := &fakeReplyEnqueuer{}
	res, appErr := NewService(repo, WithReplyEnqueuer(enq)).Cancel(context.Background(), 7, CancelInput{ConversationID: 3, RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("Cancel returned error: %v", appErr)
	}
	if res.ConversationID != 3 || res.RequestID != "rid" || res.Status != "canceled" {
		t.Fatalf("unexpected cancel response: %#v", res)
	}
	if enq.cancelPayload.ConversationID != 3 || enq.cancelPayload.UserID != 7 || enq.cancelPayload.RequestID != "rid" {
		t.Fatalf("unexpected cancel payload: %#v", enq.cancelPayload)
	}
}

func TestSendRejectsNonChatAgent(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["image"]`}}
	_, appErr := NewService(repo, WithReplyEnqueuer(&fakeReplyEnqueuer{})).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "hello", RequestID: "rid"})
	if appErr == nil || appErr.Code != 100 || appErr.Message != "该智能体不支持对话场景" {
		t.Fatalf("expected non-chat bad request, got %#v", appErr)
	}
}
