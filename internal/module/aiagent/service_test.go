package aiagent

import (
	"context"
	"errors"
	"testing"
)

func TestServiceRejectsRetiredScenesAndBuildsInit(t *testing.T) {
	repo := &fakeRepo{models: []ModelOptionRow{{ID: 1, Name: "GPT", Driver: "openai"}}, knowledge: []OptionRow{{ID: 2, Name: "KB"}}}
	service := NewService(repo)

	init, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init error: %v", appErr)
	}
	if len(init.Dict.AISceneArr) != 0 {
		t.Fatalf("retired scenes must not be exposed: %#v", init.Dict.AISceneArr)
	}
	if len(init.Dict.ModelList) != 1 || len(init.Dict.KnowledgeBaseList) != 1 {
		t.Fatalf("unexpected init lists: %#v", init.Dict)
	}

	_, appErr = service.Create(context.Background(), MutationInput{Name: "bad", ModelID: 1, Scene: stringPtr("goods_script")})
	if appErr == nil {
		t.Fatalf("expected retired scene rejection")
	}
	_, appErr = service.Create(context.Background(), MutationInput{Name: "bad", ModelID: 1, SceneCodes: []string{"cine_project"}})
	if appErr == nil {
		t.Fatalf("expected retired scene code rejection")
	}
}

func TestServiceCreateValidatesAndSyncsBindingsInTx(t *testing.T) {
	repo := &fakeRepo{models: []ModelOptionRow{{ID: 1, Name: "GPT", Driver: "openai"}}, activeTools: map[int64]bool{3: true}, activeKnowledge: map[int64]bool{2: true}}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), MutationInput{Name: "agent", ModelID: 1, Mode: "rag", ToolIDs: []int64{3, 3}, KnowledgeBaseIDs: []int64{2}, Capabilities: Capabilities{RAG: true, Tools: true}})
	if appErr != nil {
		t.Fatalf("Create error: %v", appErr)
	}
	if id != 99 {
		t.Fatalf("unexpected id %d", id)
	}
	if !repo.txCalled || len(repo.toolIDs) != 1 || repo.toolIDs[0] != 3 || len(repo.knowledgeIDs) != 1 || repo.knowledgeIDs[0] != 2 {
		t.Fatalf("bindings not synced in tx: %#v", repo)
	}
	if repo.created.Mode != "rag" {
		t.Fatalf("mode not stored: %#v", repo.created)
	}
}

func TestServiceCreateRejectsInactiveBindings(t *testing.T) {
	repo := &fakeRepo{models: []ModelOptionRow{{ID: 1, Name: "GPT", Driver: "openai"}}, activeTools: map[int64]bool{}, activeKnowledge: map[int64]bool{}}
	service := NewService(repo)
	if _, appErr := service.Create(context.Background(), MutationInput{Name: "agent", ModelID: 1, ToolIDs: []int64{3}}); appErr == nil {
		t.Fatalf("expected inactive tool rejection")
	}
	if _, appErr := service.Create(context.Background(), MutationInput{Name: "agent", ModelID: 1, KnowledgeBaseIDs: []int64{2}}); appErr == nil {
		t.Fatalf("expected inactive knowledge rejection")
	}
}

type fakeRepo struct {
	models          []ModelOptionRow
	knowledge       []OptionRow
	activeTools     map[int64]bool
	activeKnowledge map[int64]bool
	txCalled        bool
	created         Agent
	toolIDs         []int64
	knowledgeIDs    []int64
	sceneCodes      []string
}

func (f *fakeRepo) InitModels(ctx context.Context) ([]ModelOptionRow, error) { return f.models, nil }
func (f *fakeRepo) InitKnowledgeBases(ctx context.Context) ([]OptionRow, error) {
	return f.knowledge, nil
}
func (f *fakeRepo) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Get(ctx context.Context, id int64) (*Agent, error) {
	if id == 1 {
		return &Agent{ID: 1, Name: "agent", ModelID: 1, Mode: "chat", Status: 1, IsDel: 2}, nil
	}
	return nil, nil
}
func (f *fakeRepo) ActiveModelExists(ctx context.Context, id int64) (bool, error) {
	for _, m := range f.models {
		if m.ID == id {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeRepo) ActiveToolIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	out := map[int64]struct{}{}
	for _, id := range ids {
		if f.activeTools[id] {
			out[id] = struct{}{}
		}
	}
	return out, nil
}
func (f *fakeRepo) ActiveKnowledgeBaseIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	out := map[int64]struct{}{}
	for _, id := range ids {
		if f.activeKnowledge[id] {
			out[id] = struct{}{}
		}
	}
	return out, nil
}
func (f *fakeRepo) BindingData(ctx context.Context, agentIDs []int64) (BindingData, error) {
	return BindingData{}, nil
}
func (f *fakeRepo) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.txCalled = true
	return fn(f)
}
func (f *fakeRepo) CreateAgent(ctx context.Context, row Agent) (int64, error) {
	f.created = row
	return 99, nil
}
func (f *fakeRepo) UpdateAgent(ctx context.Context, id int64, fields map[string]any) error {
	return nil
}
func (f *fakeRepo) ChangeStatus(ctx context.Context, id int64, status int) error   { return nil }
func (f *fakeRepo) SoftDeleteAgentAndBindings(ctx context.Context, id int64) error { return nil }
func (f *fakeRepo) SyncToolBindings(ctx context.Context, agentID int64, toolIDs []int64) error {
	f.toolIDs = toolIDs
	return nil
}
func (f *fakeRepo) SyncKnowledgeBindings(ctx context.Context, agentID int64, knowledgeIDs []int64) error {
	f.knowledgeIDs = knowledgeIDs
	return nil
}
func (f *fakeRepo) SyncSceneBindings(ctx context.Context, agentID int64, sceneCodes []string) error {
	f.sceneCodes = sceneCodes
	return nil
}

var _ Repository = (*fakeRepo)(nil)

func stringPtr(value string) *string { return &value }

var _ = errors.New
