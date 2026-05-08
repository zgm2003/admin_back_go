package aiprompt

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows        []Prompt
	total       int64
	rowByID     map[int64]Prompt
	created     *Prompt
	updates     []map[string]any
	deletedID   int64
	incrementID int64
	listQuery   ListQuery
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Prompt, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Prompt) (int64, error) {
	f.created = &row
	return 31, nil
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int64) error {
	f.deletedID = id
	return nil
}

func (f *fakeRepository) IncrementUseCount(ctx context.Context, id int64) error {
	f.incrementID = id
	return nil
}

func TestPromptModelUsesCanonicalTableOnly(t *testing.T) {
	if table := (Prompt{}).TableName(); table != "ai_prompts" {
		t.Fatalf("P1 must use canonical ai_prompts table, got %q", table)
	}
}

func TestListScopesToCurrentUserAndReturnsArrays(t *testing.T) {
	createdAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []Prompt{{
		ID: 1, UserID: 9, Title: "标题", Content: "内容", Category: "ops",
		Tags: `["a","b"]`, Variables: strPtr(`["name","count"]`),
		IsFavorite: enum.CommonYes, UseCount: 3, Sort: 2, CreatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), 9, ListQuery{CurrentPage: 1, PageSize: 20, Title: " 标 "})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.listQuery.UserID != 9 || repo.listQuery.Title != "标" {
		t.Fatalf("list must be scoped and normalized: %#v", repo.listQuery)
	}
	if len(got.List) != 1 || !reflect.DeepEqual(got.List[0].Tags, []string{"a", "b"}) || !reflect.DeepEqual(got.List[0].Variables, []string{"name", "count"}) {
		t.Fatalf("tags/variables must be arrays: %#v", got.List)
	}
}

func TestDetailRejectsOtherUser(t *testing.T) {
	repo := &fakeRepository{rowByID: map[int64]Prompt{5: {ID: 5, UserID: 8, Title: "other"}}}
	service := NewService(repo)

	_, appErr := service.Detail(context.Background(), 9, 5)
	if appErr == nil || appErr.Code != apperror.CodeForbidden || appErr.Message != "无权访问" {
		t.Fatalf("expected ownership error, got %#v", appErr)
	}
}

func TestCreateSetsCurrentUserDefaultsAndSerializesArrays(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), 9, CreateInput{
		Title: " 标题 ", Content: " 内容 ", Category: " ops ", Tags: []string{" a ", "", "b"}, Variables: []string{"name", " count "},
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 31 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.UserID != 9 || repo.created.IsFavorite != enum.CommonNo || repo.created.UseCount != 0 || repo.created.Sort != 0 || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("defaults/user scope wrong: %#v", repo.created)
	}
	if repo.created.Tags != `["a","b"]` || repo.created.Variables == nil || *repo.created.Variables != `["name","count"]` {
		t.Fatalf("arrays not serialized canonically: %#v", repo.created)
	}
}

func TestUpdateDeleteToggleAndUseRequireOwner(t *testing.T) {
	repo := &fakeRepository{rowByID: map[int64]Prompt{5: {ID: 5, UserID: 9, IsFavorite: enum.CommonNo, Content: "内容"}}}
	service := NewService(repo)

	appErr := service.Update(context.Background(), 9, 5, UpdateInput{Title: "新", Content: "内容", Tags: []string{"x"}, Variables: []string{"v"}})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	variables, ok := repo.updates[0]["variables"].(*string)
	if len(repo.updates) != 1 || repo.updates[0]["tags"] != `["x"]` || !ok || variables == nil || *variables != `["v"]` {
		t.Fatalf("unexpected update fields: %#v", repo.updates)
	}

	favorite, appErr := service.ToggleFavorite(context.Background(), 9, 5)
	if appErr != nil || favorite.IsFavorite != enum.CommonYes {
		t.Fatalf("expected favorite flip to yes, got %#v err=%v", favorite, appErr)
	}
	use, appErr := service.Use(context.Background(), 9, 5)
	if appErr != nil || use.Content != "内容" || repo.incrementID != 5 {
		t.Fatalf("expected use to increment and return content, use=%#v err=%v repo=%#v", use, appErr, repo)
	}
	appErr = service.Delete(context.Background(), 9, 5)
	if appErr != nil || repo.deletedID != 5 {
		t.Fatalf("expected owner delete, got err=%v repo=%#v", appErr, repo)
	}

	otherRepo := &fakeRepository{rowByID: map[int64]Prompt{6: {ID: 6, UserID: 8}}}
	otherService := NewService(otherRepo)
	appErr = otherService.Delete(context.Background(), 9, 6)
	if appErr == nil || !strings.Contains(appErr.Message, "无权操作") || otherRepo.deletedID != 0 {
		t.Fatalf("other user must not delete prompt, err=%#v repo=%#v", appErr, otherRepo)
	}
}

func strPtr(value string) *string {
	return &value
}
