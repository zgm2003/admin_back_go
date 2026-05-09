package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

type fakeAIAgentRepository struct {
	rows             []AgentWithProvider
	total            int64
	rawByID          map[uint64]Agent
	rowByID          map[uint64]AgentWithProvider
	activeProviders  map[uint64]Provider
	modelsByProvider map[uint64][]ProviderModel
	connections      []Provider
	existsBinding    bool
	created          *Agent
	updates          []map[string]any
	statusID         uint64
	status           int
	deletedID        uint64
	bindings         []Binding
	createdBinding   *Binding
	deletedBindingID uint64
	visibleAgents    []Agent
}

func (f *fakeAIAgentRepository) List(ctx context.Context, query ListQuery) ([]AgentWithProvider, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeAIAgentRepository) Get(ctx context.Context, id uint64) (*AgentWithProvider, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAgentRepository) GetRaw(ctx context.Context, id uint64) (*Agent, error) {
	if f.rawByID == nil {
		return nil, nil
	}
	row, ok := f.rawByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAgentRepository) ListActiveProviders(ctx context.Context) ([]Provider, error) {
	return f.connections, nil
}

func (f *fakeAIAgentRepository) GetActiveProvider(ctx context.Context, id uint64) (*Provider, error) {
	if f.activeProviders == nil {
		return nil, nil
	}
	row, ok := f.activeProviders[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAgentRepository) ListProviderModels(ctx context.Context, providerID uint64) ([]ProviderModel, error) {
	if f.modelsByProvider == nil {
		return nil, nil
	}
	return f.modelsByProvider[providerID], nil
}

func (f *fakeAIAgentRepository) Create(ctx context.Context, row Agent) (uint64, error) {
	f.created = &row
	return 11, nil
}

func (f *fakeAIAgentRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeAIAgentRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeAIAgentRepository) Delete(ctx context.Context, id uint64) error {
	f.deletedID = id
	return nil
}

func (f *fakeAIAgentRepository) ListBindings(ctx context.Context, agentID uint64) ([]Binding, error) {
	return f.bindings, nil
}

func (f *fakeAIAgentRepository) ExistsBinding(ctx context.Context, agentID uint64, bindType string, bindKey string, excludeID uint64) (bool, error) {
	return f.existsBinding, nil
}

func (f *fakeAIAgentRepository) CreateBinding(ctx context.Context, row Binding) (uint64, error) {
	f.createdBinding = &row
	return 22, nil
}

func (f *fakeAIAgentRepository) DeleteBinding(ctx context.Context, id uint64) error {
	f.deletedBindingID = id
	return nil
}

func (f *fakeAIAgentRepository) ListVisibleAgents(ctx context.Context, query OptionQuery) ([]Agent, error) {
	return f.visibleAgents, nil
}

type fakeAIAgentTester struct {
	input platformai.TestConnectionInput
	err   error
}

func (f *fakeAIAgentTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	f.input = input
	if f.err != nil {
		return &platformai.TestConnectionResult{OK: false, Status: "500", Message: f.err.Error()}, f.err
	}
	return &platformai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
}

func TestCreateRejectsMissingActiveProvider(t *testing.T) {
	service := NewService(&fakeAIAgentRepository{}, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 99,
		Name:       "客服助手",
		ModelID:    "gpt-4.1-mini",
		Scenes:     []string{"chat"},
		Status:     enum.CommonYes,
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI供应商不存在或已禁用" {
		t.Fatalf("expected missing active connection error, got %#v", appErr)
	}
}

func TestCreateRequiresProviderModelAndDefaultScene(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {
			{ProviderID: 1, ModelID: "gpt-4.1-mini", DisplayName: "GPT-4.1 mini", Status: enum.CommonYes},
		}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID:   1,
		Name:         "客服助手",
		ModelID:      "gpt-4.1-mini",
		SystemPrompt: "你是客服助手",
		Avatar:       "https://cdn.example/avatar.png",
		Status:       enum.CommonYes,
	})

	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if repo.created == nil {
		t.Fatal("expected created agent")
	}
	if repo.created.ModelID != "gpt-4.1-mini" || repo.created.ModelDisplayName != "GPT-4.1 mini" {
		t.Fatalf("model selection not persisted: %#v", repo.created)
	}
	if repo.created.ScenesJSON != `["chat"]` {
		t.Fatalf("blank scenes must default to chat, got %s", repo.created.ScenesJSON)
	}
	if repo.created.SystemPrompt != "你是客服助手" || repo.created.Avatar != "https://cdn.example/avatar.png" {
		t.Fatalf("system prompt/avatar not persisted: %#v", repo.created)
	}
}

func TestCreateRejectsModelOutsideProviderSnapshot(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "客服助手",
		ModelID:    "gpt-4o",
		Scenes:     []string{"chat"},
		Status:     enum.CommonYes,
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "关联模型不存在或已禁用" {
		t.Fatalf("expected invalid model error, got %#v", appErr)
	}
}

func TestCreateRejectsInvalidScene(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "客服助手",
		ModelID:    "gpt-4.1-mini",
		Scenes:     []string{"chat", "rag"},
		Status:     enum.CommonYes,
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的智能体场景" {
		t.Fatalf("expected invalid scene error, got %#v", appErr)
	}
}

func TestListDTOExcludesSecretsAndOverdesignedFields(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeAIAgentRepository{
		rows: []AgentWithProvider{{
			Agent: Agent{
				ID:               1,
				ProviderID:       3,
				Name:             "客服助手",
				ModelID:          "gpt-4.1-mini",
				ModelDisplayName: "GPT-4.1 mini",
				ScenesJSON:       `["chat"]`,
				SystemPrompt:     "你是客服助手",
				Avatar:           "https://cdn.example/avatar.png",
				Status:           enum.CommonYes,
				IsDel:            enum.CommonNo,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			ProviderName: "Dify",
			EngineType:   "dify",
		}},
		total: 1,
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 {
		t.Fatalf("unexpected list response: %#v", got)
	}
	if got.List[0].ModelID != "gpt-4.1-mini" || len(got.List[0].Scenes) != 1 || got.List[0].Scenes[0] != "chat" || got.List[0].SystemPrompt != "你是客服助手" || got.List[0].Avatar == "" {
		t.Fatalf("MVP fields missing from list response: %#v", got.List[0])
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"code", "agent_type", "agent_type_name", "external_agent_id", "external_agent_api_key", "external_agent_api_key_enc", "external_agent_api_key_hint", "default_response_mode", "runtime_config", "runtime_config_json", "model_snapshot_json"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list response leaked removed agent field %q in %s", forbidden, body)
		}
	}
}

func TestCreateBindingRejectsDuplicateScope(t *testing.T) {
	repo := &fakeAIAgentRepository{
		rawByID:       map[uint64]Agent{7: {ID: 7, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		existsBinding: true,
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.CreateBinding(context.Background(), 7, BindingInput{BindType: "user", BindKey: "9", Status: enum.CommonYes})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI智能体绑定已存在" {
		t.Fatalf("expected duplicate binding error, got %#v", appErr)
	}
}

func TestOptionsExcludeDisabledAgents(t *testing.T) {
	repo := &fakeAIAgentRepository{visibleAgents: []Agent{
		{ID: 1, Name: "启用智能体", Status: enum.CommonYes, IsDel: enum.CommonNo},
		{ID: 2, Name: "禁用智能体", Status: enum.CommonNo, IsDel: enum.CommonNo},
		{ID: 3, Name: "删除智能体", Status: enum.CommonYes, IsDel: enum.CommonYes},
	}}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.Options(context.Background(), OptionQuery{UserID: 9, Platform: "admin"})
	if appErr != nil {
		t.Fatalf("expected options to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].Value != 1 {
		t.Fatalf("disabled/deleted agents must be excluded, got %#v", got.List)
	}
}

func TestUpdateOnlyPersistsMVPFields(t *testing.T) {
	repo := &fakeAIAgentRepository{
		rawByID:          map[uint64]Agent{5: {ID: 5, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "Dify", EngineType: "dify", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	appErr := service.Update(context.Background(), 5, UpdateInput{ProviderID: 1, Name: "客服助手", ModelID: "gpt-4.1-mini", Scenes: []string{"chat"}, Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one update, got %#v", repo.updates)
	}
	for _, forbidden := range []string{"code", "agent_type", "external_agent_id", "external_agent_api_key_enc", "external_agent_api_key_hint", "default_response_mode", "runtime_config_json", "model_snapshot_json", "created_by", "updated_by"} {
		if _, ok := repo.updates[0][forbidden]; ok {
			t.Fatalf("update must not write removed field %q: %#v", forbidden, repo.updates[0])
		}
	}
}

func TestTestDecryptsProviderKeyAndUsesActiveProvider(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAgentRepository{
		rowByID: map[uint64]AgentWithProvider{5: {
			Agent: Agent{ID: 5, ProviderID: 1, Status: enum.CommonYes, IsDel: enum.CommonNo},
		}},
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	tester := &fakeAIAgentTester{}
	service := NewService(repo, box, tester)

	result, appErr := service.Test(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test to succeed, got %v", appErr)
	}
	if result == nil || !result.OK {
		t.Fatalf("expected successful test result, got %#v", result)
	}
	if tester.input.APIKey != "plain-provider-key" || tester.input.BaseURL != "https://api.dify.test/v1" || tester.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected tester input: %#v", tester.input)
	}
}

func TestTestReturnsUpstreamError(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAgentRepository{
		rowByID:         map[uint64]AgentWithProvider{5: {Agent: Agent{ID: 5, ProviderID: 1, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	service := NewService(repo, box, &fakeAIAgentTester{err: errors.New("upstream failed")})

	_, appErr := service.Test(context.Background(), 5)
	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.Message != "测试AI智能体失败" {
		t.Fatalf("expected wrapped upstream error, got %#v", appErr)
	}
}
