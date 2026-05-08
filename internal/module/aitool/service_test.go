package aitool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows          []Tool
	total         int64
	rowByID       map[int64]Tool
	exists        bool
	hasBindings   bool
	activeOptions []ToolOptionRow
	boundToolIDs  []int64
	created       *Tool
	updates       []map[string]any
	statusID      int64
	status        int
	deletedID     int64
	syncAgentID   int64
	syncToolIDs   []int64
	syncCalled    bool
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Tool, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Tool, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Tool) (int64, error) {
	f.created = &row
	return 21, nil
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int64) error {
	f.deletedID = id
	return nil
}

func (f *fakeRepository) HasActiveBindings(ctx context.Context, toolID int64) (bool, error) {
	return f.hasBindings, nil
}

func (f *fakeRepository) ActiveOptions(ctx context.Context) ([]ToolOptionRow, error) {
	return f.activeOptions, nil
}

func (f *fakeRepository) BoundToolIDs(ctx context.Context, agentID int64) ([]int64, error) {
	return f.boundToolIDs, nil
}

func (f *fakeRepository) SyncBindings(ctx context.Context, agentID int64, toolIDs []int64) error {
	f.syncCalled = true
	f.syncAgentID = agentID
	f.syncToolIDs = append([]int64{}, toolIDs...)
	return nil
}

func TestInitReturnsToolDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.AIExecutorTypeArr) != 3 || got.Dict.AIExecutorTypeArr[0].Value != enum.AIExecutorInternal {
		t.Fatalf("unexpected executor dict: %#v", got.Dict.AIExecutorTypeArr)
	}
	if len(got.Dict.CommonStatusArr) != 2 || got.Dict.CommonStatusArr[0].Value != enum.CommonYes {
		t.Fatalf("unexpected status dict: %#v", got.Dict.CommonStatusArr)
	}
}

func TestCreateRejectsInvalidCode(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Create(context.Background(), CreateInput{Name: "bad", Code: "CineBad", ExecutorType: enum.AIExecutorInternal})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "工具编码格式错误" {
		t.Fatalf("expected code validation error, got %#v", appErr)
	}
}

func TestCreateRejectsDuplicateCode(t *testing.T) {
	service := NewService(&fakeRepository{exists: true})

	_, appErr := service.Create(context.Background(), CreateInput{Name: "统计", Code: "query_user_stats", ExecutorType: enum.AIExecutorInternal})
	if appErr == nil || appErr.Message != "工具编码已存在" {
		t.Fatalf("expected duplicate code error, got %#v", appErr)
	}
}

func TestCreateValidatesExecutorConfig(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Create(context.Background(), CreateInput{Name: "HTTP", Code: "http_tool", ExecutorType: enum.AIExecutorHTTPWhitelist, ExecutorConfig: map[string]any{"url": "http://example.test"}})
	if appErr == nil || appErr.Message != "HTTP白名单执行器的 URL 必须以 https:// 开头" {
		t.Fatalf("expected https whitelist error, got %#v", appErr)
	}

	_, appErr = service.Create(context.Background(), CreateInput{Name: "SQL", Code: "sql_tool", ExecutorType: enum.AIExecutorSQLReadonly, ExecutorConfig: map[string]any{"sql": "UPDATE users SET name='x'"}})
	if appErr == nil || appErr.Message != "只读SQL执行器的 SQL 必须以 SELECT 开头" {
		t.Fatalf("expected readonly sql error, got %#v", appErr)
	}
}

func TestCreateNormalizesAndStoresJSONObject(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), CreateInput{
		Name: " 查询 ", Code: "query_user_stats", Description: " 描述 ",
		SchemaJSON:     map[string]any{"type": "object"},
		ExecutorType:   enum.AIExecutorInternal,
		ExecutorConfig: map[string]any{"handler": "query_user_stats"},
		Status:         enum.CommonYes,
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 21 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Name != "查询" || repo.created.Code != "query_user_stats" || repo.created.Description != "描述" {
		t.Fatalf("fields were not normalized: %#v", repo.created)
	}
	if repo.created.SchemaJSON == nil || *repo.created.SchemaJSON != `{"type":"object"}` {
		t.Fatalf("schema json was not stored canonically: %#v", repo.created.SchemaJSON)
	}
	if repo.created.ExecutorConfig == nil || !strings.Contains(*repo.created.ExecutorConfig, `"handler":"query_user_stats"`) {
		t.Fatalf("executor config was not stored as json object: %#v", repo.created.ExecutorConfig)
	}
}

func TestDeleteRejectsActiveBindings(t *testing.T) {
	repo := &fakeRepository{hasBindings: true, rowByID: map[int64]Tool{7: {ID: 7, Name: "绑定工具"}}}
	service := NewService(repo)

	appErr := service.Delete(context.Background(), 7)
	if appErr == nil || appErr.Message != "工具已被智能体绑定，请先解绑再删除" {
		t.Fatalf("expected active binding rejection, got %#v", appErr)
	}
	if repo.deletedID != 0 {
		t.Fatalf("bound tool must not be deleted")
	}
}

func TestAgentOptionsExcludeRetiredCineToolAndSyncBindings(t *testing.T) {
	repo := &fakeRepository{
		activeOptions: []ToolOptionRow{
			{ID: 1, Name: "统计", Code: "query_user_stats"},
			{ID: 13, Name: "短剧关键帧", Code: "cine_generate_keyframe"},
		},
		boundToolIDs: []int64{1, 13},
	}
	service := NewService(repo)

	got, appErr := service.AgentOptions(context.Background(), 8)
	if appErr != nil {
		t.Fatalf("expected options to succeed, got %v", appErr)
	}
	if len(got.AllTools) != 1 || got.AllTools[0].Code != "query_user_stats" {
		t.Fatalf("retired cine tool must not be returned: %#v", got.AllTools)
	}
	if len(got.BoundToolIDs) != 1 || got.BoundToolIDs[0] != 1 {
		t.Fatalf("retired cine binding must not be exposed: %#v", got.BoundToolIDs)
	}

	appErr = service.SyncAgentBindings(context.Background(), 8, []int64{1, 1, 3})
	if appErr != nil {
		t.Fatalf("expected sync to succeed, got %v", appErr)
	}
	if !repo.syncCalled || repo.syncAgentID != 8 || len(repo.syncToolIDs) != 2 || repo.syncToolIDs[0] != 1 || repo.syncToolIDs[1] != 3 {
		t.Fatalf("expected normalized binding sync, repo=%#v", repo)
	}
}

func TestAgentOptionsWithoutAgentIDReturnsEmptyBoundToolIDsArray(t *testing.T) {
	repo := &fakeRepository{
		activeOptions: []ToolOptionRow{{ID: 1, Name: "统计", Code: "query_user_stats"}},
	}
	service := NewService(repo)

	got, appErr := service.AgentOptions(context.Background(), 0)
	if appErr != nil {
		t.Fatalf("expected options to succeed, got %v", appErr)
	}
	if got.BoundToolIDs == nil {
		t.Fatalf("bound_tool_ids must be an empty array, not nil")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal agent options: %v", err)
	}
	if !strings.Contains(string(encoded), `"bound_tool_ids":[]`) {
		t.Fatalf("bound_tool_ids must encode as []: %s", string(encoded))
	}
}

func TestListDTOParsesJSONObjects(t *testing.T) {
	createdAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []Tool{{
		ID: 1, Name: "统计", Code: "query_user_stats", Description: "desc", SchemaJSON: strPtr(`{"type":"object"}`),
		ExecutorType: enum.AIExecutorInternal, ExecutorConfig: strPtr(`{"handler":"query_user_stats"}`),
		Status: enum.CommonYes, CreatedAt: createdAt, UpdatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].ExecutorName != "内置函数" || got.List[0].SchemaJSON["type"] != "object" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	if !strings.Contains(string(encoded), `"schema_json":{"type":"object"}`) {
		t.Fatalf("json object shape not preserved: %s", string(encoded))
	}
}

func strPtr(value string) *string {
	return &value
}
