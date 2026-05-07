package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

type fakeRepository struct {
	contactsConfirmed         bool
	targetUser                *UserBrief
	contactRow                *Contact
	contactExists             bool
	deletedContactPair        bool
	privateConversationClosed bool
	existingPrivate           *Conversation
	createdPrivateID          int64
	createdPendingContactPair bool
	confirmedContactPair      bool
	conversationRow           *ConversationRow
	activeUserIDs             []int64
	participantOK             bool
	lastMessageID             int64
	createdMessage            *Message
	messages                  []Message
	senders                   map[int64]UserBrief
	lastReadUpdated           int64
	publishedRead             bool
}

type fakePublisher struct {
	publications []platformrealtime.Publication
	err          error
}

func (f *fakePublisher) Publish(ctx context.Context, publication platformrealtime.Publication) error {
	f.publications = append(f.publications, publication)
	return f.err
}

func (f *fakeRepository) ListConversations(ctx context.Context, query ConversationListQuery) ([]ConversationRow, int64, error) {
	if f.conversationRow == nil {
		return nil, 0, nil
	}
	return []ConversationRow{*f.conversationRow}, 1, nil
}

func (f *fakeRepository) ConversationUnreadCounts(ctx context.Context, userID int64, conversationIDs []int64) (map[int64]int64, error) {
	return map[int64]int64{10: 3}, nil
}

func (f *fakeRepository) PrivateConversationPeers(ctx context.Context, conversationIDs []int64, currentUserID int64) (map[int64]UserBrief, error) {
	return map[int64]UserBrief{10: {ID: 2, Username: "peer", Avatar: "peer.png"}}, nil
}

func (f *fakeRepository) ListContacts(ctx context.Context, userID int64) ([]ContactRow, error) {
	return nil, nil
}

func (f *fakeRepository) FindActiveUser(ctx context.Context, userID int64) (*UserBrief, error) {
	return f.targetUser, nil
}

func (f *fakeRepository) IsConfirmedContact(ctx context.Context, userID int64, contactUserID int64) (bool, error) {
	return f.contactsConfirmed, nil
}

func (f *fakeRepository) FindContact(ctx context.Context, userID int64, contactUserID int64) (*Contact, error) {
	return f.contactRow, nil
}

func (f *fakeRepository) ContactExists(ctx context.Context, userIDA int64, userIDB int64) (bool, error) {
	return f.contactExists, nil
}

func (f *fakeRepository) CreatePendingContactPair(ctx context.Context, initiatorID int64, targetID int64, now time.Time) error {
	f.createdPendingContactPair = true
	return nil
}

func (f *fakeRepository) ConfirmContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	f.confirmedContactPair = true
	return nil
}

func (f *fakeRepository) SoftDeleteContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	f.deletedContactPair = true
	return nil
}

func (f *fakeRepository) ClosePrivateConversationForContactPair(ctx context.Context, userIDA int64, userIDB int64, now time.Time) error {
	f.privateConversationClosed = true
	return nil
}

func (f *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(f)
}

func (f *fakeRepository) LockConfirmedContactPair(ctx context.Context, userIDA int64, userIDB int64) error {
	return nil
}

func (f *fakeRepository) FindPrivateConversation(ctx context.Context, userIDA int64, userIDB int64) (*Conversation, error) {
	return f.existingPrivate, nil
}

func (f *fakeRepository) CreatePrivateConversation(ctx context.Context, userIDA int64, userIDB int64, now time.Time) (*Conversation, error) {
	f.createdPrivateID = 99
	return &Conversation{ID: 99, Type: ConversationTypePrivate, MemberCount: 2, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now, LastMessageAt: now}, nil
}

func (f *fakeRepository) RestoreParticipant(ctx context.Context, conversationID int64, userID int64) error {
	return nil
}

func (f *fakeRepository) FindConversationRow(ctx context.Context, conversationID int64, currentUserID int64) (*ConversationRow, error) {
	return f.conversationRow, nil
}

func (f *fakeRepository) IsActiveParticipant(ctx context.Context, conversationID int64, userID int64) (bool, error) {
	return f.participantOK, nil
}

func (f *fakeRepository) ActiveParticipantUserIDs(ctx context.Context, conversationID int64) ([]int64, error) {
	return f.activeUserIDs, nil
}

func (f *fakeRepository) CreateMessage(ctx context.Context, input CreateMessageInput) (*Message, error) {
	f.createdMessage = &Message{ID: 77, ConversationID: input.ConversationID, SenderID: input.SenderID, Type: input.Type, Content: input.Content, MetaJSON: input.MetaJSON, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt, IsDel: enum.CommonNo}
	return f.createdMessage, nil
}

func (f *fakeRepository) UpdateConversationLastMessage(ctx context.Context, conversationID int64, messageID int64, messageAt time.Time, preview string) error {
	f.lastMessageID = messageID
	return nil
}

func (f *fakeRepository) FindUserBrief(ctx context.Context, userID int64) (*UserBrief, error) {
	if f.targetUser != nil && f.targetUser.ID == userID {
		return f.targetUser, nil
	}
	return &UserBrief{ID: userID, Username: "user", Avatar: "avatar.png"}, nil
}

func (f *fakeRepository) ListMessages(ctx context.Context, query MessageListQuery) ([]Message, int64, error) {
	return f.messages, int64(len(f.messages)), nil
}

func (f *fakeRepository) UserBriefs(ctx context.Context, userIDs []int64) (map[int64]UserBrief, error) {
	return f.senders, nil
}

func (f *fakeRepository) ConversationLastMessageID(ctx context.Context, conversationID int64) (int64, error) {
	return 77, nil
}

func (f *fakeRepository) UpdateLastReadMessageID(ctx context.Context, conversationID int64, userID int64, messageID int64) error {
	f.lastReadUpdated = messageID
	return nil
}

func (f *fakeRepository) SoftDeleteConversationForUser(ctx context.Context, conversationID int64, userID int64) (int64, error) {
	return 1, nil
}

func (f *fakeRepository) TogglePin(ctx context.Context, conversationID int64, userID int64) error {
	return nil
}

func TestSendMessagePublishesVersionedEventAfterPersisting(t *testing.T) {
	repo := &fakeRepository{participantOK: true, activeUserIDs: []int64{1, 2}, targetUser: &UserBrief{ID: 1, Username: "admin", Avatar: "a.png"}}
	publisher := &fakePublisher{}
	service := NewService(repo, WithRealtimePublisher(publisher))

	got, appErr := service.SendMessage(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, SendMessageInput{ConversationID: 10, Type: MessageTypeText, Content: " hello "})
	if appErr != nil {
		t.Fatalf("SendMessage returned app error: %v", appErr)
	}
	if got.Message.ID != 77 || got.Message.Sender == nil || got.Message.Sender.Username != "admin" {
		t.Fatalf("unexpected message response: %#v", got.Message)
	}
	if repo.lastMessageID != 77 {
		t.Fatalf("last message not updated, got %d", repo.lastMessageID)
	}
	if len(publisher.publications) != 2 {
		t.Fatalf("expected publish to all active participants, got %#v", publisher.publications)
	}
	first := publisher.publications[0]
	if first.Platform != enum.PlatformAdmin || first.UserID != 1 || first.Envelope.Type != EventChatMessageCreatedV1 {
		t.Fatalf("unexpected first publication: %#v", first)
	}
	var payload map[string]any
	if err := json.Unmarshal(first.Envelope.Data, &payload); err != nil {
		t.Fatalf("invalid realtime payload: %v", err)
	}
	if payload["conversation_id"] != float64(10) {
		t.Fatalf("unexpected realtime payload: %#v", payload)
	}
}

func TestSendMessageRejectsNonParticipant(t *testing.T) {
	repo := &fakeRepository{participantOK: false}
	service := NewService(repo)

	_, appErr := service.SendMessage(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, SendMessageInput{ConversationID: 10, Type: MessageTypeText, Content: "hello"})
	if appErr == nil || appErr.Code != apperror.CodeForbidden {
		t.Fatalf("expected forbidden app error, got %#v", appErr)
	}
}

func TestCreatePrivateRequiresConfirmedContact(t *testing.T) {
	repo := &fakeRepository{contactsConfirmed: false, targetUser: &UserBrief{ID: 2, Username: "peer"}}
	service := NewService(repo)

	_, appErr := service.CreatePrivate(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, CreatePrivateInput{UserID: 2})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request when contact is not confirmed, got %#v", appErr)
	}
}

func TestAddContactCreatesPendingBidirectionalRequest(t *testing.T) {
	repo := &fakeRepository{targetUser: &UserBrief{ID: 2, Username: "peer"}, contactExists: false}
	service := NewService(repo)

	appErr := service.AddContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr != nil {
		t.Fatalf("AddContact returned app error: %v", appErr)
	}
	if !repo.createdPendingContactPair {
		t.Fatalf("expected pending contact pair to be created")
	}
}

func TestAddContactRejectsExistingContactOrPendingRequest(t *testing.T) {
	repo := &fakeRepository{targetUser: &UserBrief{ID: 2, Username: "peer"}, contactExists: true}
	service := NewService(repo)

	appErr := service.AddContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request for existing contact, got %#v", appErr)
	}
}

func TestConfirmContactRequiresIncomingPendingRequest(t *testing.T) {
	repo := &fakeRepository{contactRow: &Contact{UserID: 1, ContactUserID: 2, Status: ContactStatusPending, IsInitiator: enum.CommonNo, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.ConfirmContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr != nil {
		t.Fatalf("ConfirmContact returned app error: %v", appErr)
	}
	if !repo.confirmedContactPair {
		t.Fatalf("expected contact pair to be confirmed")
	}
}

func TestConfirmContactRejectsOwnOutgoingRequest(t *testing.T) {
	repo := &fakeRepository{contactRow: &Contact{UserID: 1, ContactUserID: 2, Status: ContactStatusPending, IsInitiator: enum.CommonYes, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.ConfirmContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request for outgoing request confirm, got %#v", appErr)
	}
}

func TestDeleteContactSoftDeletesBidirectionalRows(t *testing.T) {
	repo := &fakeRepository{contactRow: &Contact{UserID: 1, ContactUserID: 2, Status: ContactStatusConfirmed, IsInitiator: enum.CommonYes, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.DeleteContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr != nil {
		t.Fatalf("DeleteContact returned app error: %v", appErr)
	}
	if !repo.deletedContactPair {
		t.Fatalf("expected contact pair to be soft deleted")
	}
	if !repo.privateConversationClosed {
		t.Fatalf("expected confirmed contact delete to close private conversation")
	}
}

func TestDeletePendingContactDoesNotClosePrivateConversation(t *testing.T) {
	repo := &fakeRepository{contactRow: &Contact{UserID: 1, ContactUserID: 2, Status: ContactStatusPending, IsInitiator: enum.CommonNo, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.DeleteContact(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, ContactInput{UserID: 2})
	if appErr != nil {
		t.Fatalf("DeleteContact returned app error: %v", appErr)
	}
	if !repo.deletedContactPair {
		t.Fatalf("expected contact pair to be soft deleted")
	}
	if repo.privateConversationClosed {
		t.Fatalf("pending request delete must not close private conversation")
	}
}

func TestMarkReadUpdatesLastReadAndPublishesReceipt(t *testing.T) {
	repo := &fakeRepository{participantOK: true, activeUserIDs: []int64{1, 2}}
	publisher := &fakePublisher{}
	service := NewService(repo, WithRealtimePublisher(publisher))

	appErr := service.MarkRead(context.Background(), Identity{UserID: 1, Platform: enum.PlatformAdmin}, MarkReadInput{ConversationID: 10})
	if appErr != nil {
		t.Fatalf("MarkRead returned app error: %v", appErr)
	}
	if repo.lastReadUpdated != 77 {
		t.Fatalf("expected last read id 77, got %d", repo.lastReadUpdated)
	}
	if len(publisher.publications) != 2 || publisher.publications[0].Envelope.Type != EventChatReadV1 {
		t.Fatalf("expected chat.read.v1 publications, got %#v", publisher.publications)
	}
}
