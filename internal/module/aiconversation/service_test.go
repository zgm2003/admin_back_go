package aiconversation

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows         []ListRow
	row          *Conversation
	activeAgents map[int64]bool
	listQuery    ListQuery
	created      Conversation
	deleteID     int64
	deleteUserID int64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, bool, error) {
	f.listQuery = query
	return f.rows, len(f.rows) > query.Limit, nil
}
func (f *fakeRepository) Get(ctx context.Context, id int64) (*Conversation, string, error) {
	if f.row == nil {
		return nil, "", nil
	}
	return f.row, "客服助手", nil
}
func (f *fakeRepository) ActiveChatAgentExists(ctx context.Context, id int64) (bool, error) {
	return f.activeAgents[id], nil
}
func (f *fakeRepository) Create(ctx context.Context, row Conversation) (int64, error) {
	f.created = row
	return 9, nil
}
func (f *fakeRepository) Delete(ctx context.Context, id int64, userID int64) error {
	f.deleteID = id
	f.deleteUserID = userID
	return nil
}

func TestListUsesCursorLimitAndDoesNotExposeUserOrStatus(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []ListRow{{Conversation: Conversation{ID: 1, UserID: 7, AgentID: 3, Title: "hello", LastMessageAt: &now, IsDel: enum.CommonNo, UpdatedAt: now}, AgentName: "客服助手"}}}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{AgentID: ptrInt64(3), BeforeID: 20, Limit: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.UserID != 7 || repo.listQuery.BeforeID != 20 || repo.listQuery.Limit != 20 || repo.listQuery.AgentID == nil || *repo.listQuery.AgentID != 3 {
		t.Fatalf("unexpected normalized query: %#v", repo.listQuery)
	}
	if len(res.List) != 1 || res.List[0].AgentName != "客服助手" || res.List[0].LastMessageAt == "" || res.NextID != 0 || res.HasMore {
		t.Fatalf("unexpected list response: %#v", res)
	}
}

func TestDetailRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 8, AgentID: 1, Title: "other", IsDel: enum.CommonNo}}
	_, appErr := NewService(repo).Detail(context.Background(), 7, 3)
	if appErr == nil || appErr.Code != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestCreateValidatesChatAgentAndSetsCurrentUser(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: true}}
	id, appErr := NewService(repo).Create(context.Background(), 7, CreateInput{AgentID: 5, Title: "  New chat  "})
	if appErr != nil {
		t.Fatalf("Create returned error: %v", appErr)
	}
	if id != 9 || repo.created.UserID != 7 || repo.created.AgentID != 5 || repo.created.Title != "New chat" || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("unexpected created row: id=%d row=%#v", id, repo.created)
	}
}

func TestCreateRejectsNonChatAgent(t *testing.T) {
	repo := &fakeRepository{activeAgents: map[int64]bool{5: false}}
	_, appErr := NewService(repo).Create(context.Background(), 7, CreateInput{AgentID: 5})
	if appErr == nil || appErr.Code != 100 || appErr.Message != "该智能体不支持对话场景" {
		t.Fatalf("expected non-chat bad request, got %#v", appErr)
	}
}

func TestDeleteRequiresOwnerAndSoftDeletesMessages(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 7, IsDel: enum.CommonNo}}
	if appErr := NewService(repo).Delete(context.Background(), 7, 3); appErr != nil {
		t.Fatalf("Delete returned error: %v", appErr)
	}
	if repo.deleteID != 3 || repo.deleteUserID != 7 {
		t.Fatalf("unexpected delete call: id=%d user=%d", repo.deleteID, repo.deleteUserID)
	}
}

func ptrInt64(v int64) *int64 { return &v }
