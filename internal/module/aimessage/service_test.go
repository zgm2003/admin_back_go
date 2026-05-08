package aimessage

import (
	"context"
	"testing"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	conversation               *Conversation
	message                    *Message
	rows                       []Message
	total                      int64
	listQuery                  ListQuery
	updatedID                  int64
	updatedContent             string
	deletedAfterConversationID int64
	deletedAfterMessageID      int64
	metaID                     int64
	metaJSON                   *string
	deleteIDs                  []int64
}

func (f *fakeRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) Message(ctx context.Context, id int64) (*Message, error) {
	return f.message, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Message, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}
func (f *fakeRepository) UpdateContent(ctx context.Context, id int64, content string) error {
	f.updatedID = id
	f.updatedContent = content
	return nil
}
func (f *fakeRepository) DeleteAfterMessage(ctx context.Context, conversationID int64, messageID int64) (int64, error) {
	f.deletedAfterConversationID = conversationID
	f.deletedAfterMessageID = messageID
	return 2, nil
}
func (f *fakeRepository) UpdateMeta(ctx context.Context, id int64, metaJSON *string) error {
	f.metaID = id
	f.metaJSON = metaJSON
	return nil
}
func (f *fakeRepository) DeleteMessages(ctx context.Context, ids []int64, userID int64) (int64, error) {
	f.deleteIDs = ids
	return int64(len(ids)), nil
}

func TestListChecksConversationOwnershipAndRoleFilter(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []Message{{ID: 1, ConversationID: 3, Role: enum.AIMessageRoleUser, Content: "hi"}}, total: 1}
	role := enum.AIMessageRoleUser
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3, Role: &role})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.ConversationID != 3 || repo.listQuery.Role == nil || *repo.listQuery.Role != role || repo.listQuery.CurrentPage != 1 {
		t.Fatalf("unexpected list query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].Content != "hi" {
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

func TestEditContentOnlyAllowsOwnedUserMessageAndDeletesLaterMessages(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, message: &Message{ID: 9, ConversationID: 3, Role: enum.AIMessageRoleUser, Content: "old"}}
	res, appErr := NewService(repo).EditContent(context.Background(), 7, 9, " next ")
	if appErr != nil {
		t.Fatalf("EditContent returned error: %v", appErr)
	}
	if repo.updatedID != 9 || repo.updatedContent != "next" {
		t.Fatalf("unexpected update: id=%d content=%q", repo.updatedID, repo.updatedContent)
	}
	if repo.deletedAfterConversationID != 3 || repo.deletedAfterMessageID != 9 || res.DeletedCount != 2 {
		t.Fatalf("unexpected delete-after: conv=%d msg=%d res=%#v", repo.deletedAfterConversationID, repo.deletedAfterMessageID, res)
	}
}

func TestEditContentRejectsAssistantMessage(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, message: &Message{ID: 9, ConversationID: 3, Role: enum.AIMessageRoleAssistant}}
	_, appErr := NewService(repo).EditContent(context.Background(), 7, 9, "x")
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad request, got %#v", appErr)
	}
}

func TestFeedbackWritesAndRemovesMetaFeedback(t *testing.T) {
	raw := `{"keep":true}`
	feedback := 2
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, message: &Message{ID: 9, ConversationID: 3, Role: enum.AIMessageRoleAssistant, MetaJSON: &raw}}
	if appErr := NewService(repo).Feedback(context.Background(), 7, 9, &feedback); appErr != nil {
		t.Fatalf("Feedback returned error: %v", appErr)
	}
	if repo.metaJSON == nil || *repo.metaJSON != `{"feedback":2,"keep":true}` {
		t.Fatalf("unexpected feedback meta: %#v", repo.metaJSON)
	}
	if appErr := NewService(repo).Feedback(context.Background(), 7, 9, nil); appErr != nil {
		t.Fatalf("remove Feedback returned error: %v", appErr)
	}
	if repo.metaJSON == nil || *repo.metaJSON != `{"keep":true}` {
		t.Fatalf("unexpected removed feedback meta: %#v", repo.metaJSON)
	}
}

func TestDeleteDeduplicatesAndLimitsBatch(t *testing.T) {
	repo := &fakeRepository{}
	ids := []int64{3, 3, 0, 2}
	res, appErr := NewService(repo).Delete(context.Background(), 7, ids)
	if appErr != nil {
		t.Fatalf("Delete returned error: %v", appErr)
	}
	if len(repo.deleteIDs) != 2 || repo.deleteIDs[0] != 2 || repo.deleteIDs[1] != 3 || res.Affected != 2 {
		t.Fatalf("unexpected delete: ids=%#v res=%#v", repo.deleteIDs, res)
	}

	tooMany := make([]int64, 101)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, appErr = NewService(repo).Delete(context.Background(), 7, tooMany)
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected too many bad request, got %#v", appErr)
	}
}
