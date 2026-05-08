package aiconversation

import (
	"context"
	"testing"

	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows         []ListRow
	total        int64
	row          *Conversation
	activeApps   map[int64]bool
	listQuery    ListQuery
	created      Conversation
	updateID     int64
	updateFields map[string]any
	statusID     int64
	statusValue  int
	deleteID     int64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}
func (f *fakeRepository) Get(ctx context.Context, id int64) (*Conversation, error) { return f.row, nil }
func (f *fakeRepository) AppName(ctx context.Context, id int64) (string, error) {
	return "app", nil
}
func (f *fakeRepository) ActiveAppExists(ctx context.Context, id int64) (bool, error) {
	return f.activeApps[id], nil
}
func (f *fakeRepository) Create(ctx context.Context, row Conversation) (int64, error) {
	f.created = row
	return 9, nil
}
func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updateID = id
	f.updateFields = fields
	return nil
}
func (f *fakeRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	f.statusID = id
	f.statusValue = status
	return nil
}
func (f *fakeRepository) Delete(ctx context.Context, id int64) error { f.deleteID = id; return nil }

func TestListScopesToCurrentUserAndDefaultsStatus(t *testing.T) {
	repo := &fakeRepository{rows: []ListRow{{Conversation: Conversation{ID: 1, UserID: 7, AppID: 3, Title: "hello", Status: enum.CommonYes}, AppName: "app"}}, total: 1}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{CurrentPage: 0, PageSize: 0})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.UserID != 7 || repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 20 {
		t.Fatalf("unexpected normalized query: %#v", repo.listQuery)
	}
	if repo.listQuery.Status == nil || *repo.listQuery.Status != enum.CommonYes {
		t.Fatalf("expected default status=1, got %#v", repo.listQuery.Status)
	}
	if len(res.List) != 1 || res.List[0].AppID != 3 || res.List[0].AppName != "app" || res.List[0].AgentID != 3 || res.List[0].AgentName != "app" {
		t.Fatalf("unexpected list response: %#v", res)
	}
}

func TestDetailRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 8, AppID: 1, Title: "other", Status: enum.CommonYes}}
	_, appErr := NewService(repo).Detail(context.Background(), 7, 3)
	if appErr == nil || appErr.Code != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestCreateValidatesActiveAppAndSetsCurrentUser(t *testing.T) {
	repo := &fakeRepository{activeApps: map[int64]bool{5: true}}
	id, appErr := NewService(repo).Create(context.Background(), 7, MutationInput{AppID: 5, Title: "  New chat  "})
	if appErr != nil {
		t.Fatalf("Create returned error: %v", appErr)
	}
	if id != 9 || repo.created.UserID != 7 || repo.created.AppID != 5 || repo.created.Title != "New chat" || repo.created.Status != enum.CommonYes || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("unexpected created row: id=%d row=%#v", id, repo.created)
	}
}

func TestCreateAcceptsLegacyAgentIDAsAppID(t *testing.T) {
	repo := &fakeRepository{activeApps: map[int64]bool{9: true}}
	_, appErr := NewService(repo).Create(context.Background(), 7, MutationInput{AgentID: 9, Title: "legacy"})
	if appErr != nil {
		t.Fatalf("Create returned error: %v", appErr)
	}
	if repo.created.AppID != 9 {
		t.Fatalf("legacy agent_id should map to app_id, got %d", repo.created.AppID)
	}
}

func TestCreateRejectsInactiveApp(t *testing.T) {
	repo := &fakeRepository{activeApps: map[int64]bool{5: false}}
	_, appErr := NewService(repo).Create(context.Background(), 7, MutationInput{AppID: 5})
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad request, got %#v", appErr)
	}
}

func TestUpdateStatusAndDeleteRequireOwner(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 7, AppID: 1, Title: "old", Status: enum.CommonYes}}
	service := NewService(repo)
	if appErr := service.Update(context.Background(), 7, 3, MutationInput{Title: " next "}); appErr != nil {
		t.Fatalf("Update returned error: %v", appErr)
	}
	if repo.updateID != 3 || repo.updateFields["title"] != "next" {
		t.Fatalf("unexpected update: id=%d fields=%#v", repo.updateID, repo.updateFields)
	}
	if appErr := service.ChangeStatus(context.Background(), 7, 3, enum.CommonNo); appErr != nil {
		t.Fatalf("ChangeStatus returned error: %v", appErr)
	}
	if repo.statusID != 3 || repo.statusValue != enum.CommonNo {
		t.Fatalf("unexpected status update: id=%d value=%d", repo.statusID, repo.statusValue)
	}
	if appErr := service.Delete(context.Background(), 7, 3); appErr != nil {
		t.Fatalf("Delete returned error: %v", appErr)
	}
	if repo.deleteID != 3 {
		t.Fatalf("unexpected delete id: %d", repo.deleteID)
	}
}

func TestChangeStatusRejectsInvalidStatus(t *testing.T) {
	repo := &fakeRepository{row: &Conversation{ID: 3, UserID: 7}}
	appErr := NewService(repo).ChangeStatus(context.Background(), 7, 3, 99)
	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad request, got %#v", appErr)
	}
}
