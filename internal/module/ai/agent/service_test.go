package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"
)

type countingUploadRuleResolver struct {
	calls int
	rule  uploadpolicy.Rule
	err   error
}

func (resolver *countingUploadRuleResolver) ResolveActive(context.Context) (uploadpolicy.Rule, error) {
	resolver.calls++
	return resolver.rule, resolver.err
}

func TestAgentReadEndpointsResolveUploadRuleOncePerRequest(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service)
	}{
		{name: "page init", call: func(service *Service) { _, _ = service.PageInit(context.Background()) }},
		{name: "list", call: func(service *Service) {
			_, _ = service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
		}},
		{name: "detail", call: func(service *Service) { _, _ = service.Detail(context.Background(), 7) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rules := &countingUploadRuleResolver{rule: uploadpolicy.Rule{
				MaxFileBytes: 100 << 20, FileExtensions: []string{"pdf"},
			}}
			service := NewService(
				&fakeAIAgentRepository{},
				secretbox.New([]byte("12345678901234567890123456789012")),
				nil,
				WithUploadRuleResolver(rules),
			)
			test.call(service)
			if rules.calls != 1 {
				t.Fatalf("active upload rule calls=%d", rules.calls)
			}
		})
	}
}

func TestAgentContractHasNoConfigurableMaxOutputTokens(t *testing.T) {
	for _, value := range []any{Agent{}, CreateInput{}, InitDict{}, AgentDTO{}} {
		typeOf := reflect.TypeOf(value)
		if _, exists := typeOf.FieldByName("MaxOutputTokens"); exists {
			t.Errorf("%s still exposes MaxOutputTokens", typeOf.Name())
		}
	}
	body, err := os.ReadFile(filepath.Join("transport", "admin", "request.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "max_output_tokens") {
		t.Fatal("Agent mutation request still accepts max_output_tokens")
	}
}

type fakeAIAgentRepository struct {
	rows             []AgentWithProvider
	total            int64
	rawByID          map[uint64]Agent
	rowByID          map[uint64]AgentWithProvider
	activeProviders  map[uint64]Provider
	modelsByProvider map[uint64][]ProviderModel
	connections      []Provider
	created          *Agent
	updates          []map[string]any
	statusID         uint64
	status           int
	deletedID        uint64
	visibleAgents    []AgentWithProvider
	listQuery        ListQuery
	optionQuery      OptionQuery
}

func (f *fakeAIAgentRepository) List(ctx context.Context, query ListQuery) ([]AgentWithProvider, int64, error) {
	f.listQuery = query
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
	rows := append([]ProviderModel(nil), f.modelsByProvider[providerID]...)
	for index := range rows {
		if rows[index].MappingStatus != "" {
			continue
		}
		officialID, catalogVersion := rows[index].ModelID, "test-catalog"
		mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
		rows[index].MappingStatus = officialmodel.MappingStatusMapped
		rows[index].OfficialModelID = &officialID
		rows[index].OfficialCatalogVersion = &catalogVersion
		rows[index].MappedAt = &mappedAt
	}
	return rows, nil
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

func (f *fakeAIAgentRepository) ListVisibleAgents(ctx context.Context, query OptionQuery) ([]AgentWithProvider, error) {
	f.optionQuery = query
	rows := append([]AgentWithProvider(nil), f.visibleAgents...)
	for index := range rows {
		if rows[index].ModelID == "" {
			rows[index].ModelID = "test-model"
		}
		if rows[index].EngineType == "" {
			rows[index].EngineType = "openai"
		}
		if rows[index].ProviderStatus == 0 {
			rows[index].ProviderStatus = enum.CommonYes
		}
		if rows[index].ProviderModelID == 0 {
			rows[index].ProviderModelID = rows[index].ID
		}
		if rows[index].ProviderModelStatus == 0 {
			rows[index].ProviderModelStatus = enum.CommonYes
		}
		if rows[index].OfficialModelID == "" {
			rows[index].OfficialModelID = rows[index].ModelID
		}
		if rows[index].OfficialCatalogVersion == "" {
			rows[index].OfficialCatalogVersion = "test-catalog"
		}
		if rows[index].MappingStatus == "" {
			rows[index].MappingStatus = officialmodel.MappingStatusMapped
		}
	}
	return rows, nil
}

type fakeAIAgentTester struct {
	input infraai.TestConnectionInput
	err   error
}

func (f *fakeAIAgentTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	f.input = input
	if f.err != nil {
		return &infraai.TestConnectionResult{OK: false, Status: "500", Message: f.err.Error()}, f.err
	}
	return &infraai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
}

func newTestAgentService(repository Repository, box secretbox.Box, tester ConnectionTester) *Service {
	resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		rates := []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1},
		}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "test-catalog", CatalogVendor: "openai", ModelID: modelID, LifecycleStatus: officialmodel.LifecycleActive, MaxOutputTokens: pricing.MaxSafeOutputTokens},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
			PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})
	return NewService(repository, box, tester, WithPricingResolver(resolver))
}

func TestAgentSelectionAllowsOnlyMappedActiveRoutes(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{
			ProviderID: 1, ModelID: "gpt-4.1-mini", MappingStatus: officialmodel.MappingStatusUnmapped, Status: enum.CommonYes,
		}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)
	input := CreateInput{ProviderID: 1, Name: "客服助手", ModelID: "gpt-4.1-mini", Scenes: []string{"chat"}, Status: enum.CommonYes}

	if _, appErr := service.Create(context.Background(), input); appErr == nil || !strings.Contains(appErr.Message, "未映射") {
		t.Fatalf("unmapped route error=%#v", appErr)
	}
	officialID, catalogVersion := "gpt-4.1-mini", "test-catalog"
	mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repo.modelsByProvider[1][0].MappingStatus = officialmodel.MappingStatusMapped
	repo.modelsByProvider[1][0].OfficialModelID = &officialID
	repo.modelsByProvider[1][0].OfficialCatalogVersion = &catalogVersion
	repo.modelsByProvider[1][0].MappedAt = &mappedAt
	if _, appErr := service.Create(context.Background(), input); appErr != nil {
		t.Fatalf("mapped active route rejected: %v", appErr)
	}
}

func TestAgentSelectionRejectsTamperedMappedRoute(t *testing.T) {
	officialID, catalogVersion := "gpt-4.1-mini", "test-catalog"
	mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repo := &fakeAIAgentRepository{
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{
			ProviderID: 1, ModelID: "private-upstream-model", OfficialModelID: &officialID,
			OfficialCatalogVersion: &catalogVersion, MappingStatus: officialmodel.MappingStatusMapped,
			MappedAt: &mappedAt, Status: enum.CommonYes,
		}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "客服助手",
		ModelID:    "private-upstream-model",
		Scenes:     []string{"chat"},
		Status:     enum.CommonYes,
	})
	if appErr == nil {
		t.Fatal("tampered provider route was selectable")
	}
}

func TestLifecycleDeprecatedKeepsExistingCallButRejectsSelection(t *testing.T) {
	officialID, catalogVersion := "gpt-reviewed", "test-catalog"
	mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	route := ProviderModel{
		ProviderID: 1, ModelID: officialID, OfficialModelID: &officialID,
		OfficialCatalogVersion: &catalogVersion, MappingStatus: officialmodel.MappingStatusMapped,
		MappedAt: &mappedAt, Status: enum.CommonYes,
	}
	resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		return officialmodel.ResolvedModel{
			Model: officialmodel.Model{
				CatalogVersion:  catalogVersion,
				ModelID:         modelID,
				LifecycleStatus: officialmodel.LifecycleDeprecated,
			},
		}, nil
	})
	service := NewService(
		&fakeAIAgentRepository{},
		secretbox.New([]byte("12345678901234567890123456789012")),
		nil,
		WithPricingResolver(resolver),
	)

	if appErr := service.ensureOfficialModelCallable(context.Background(), route); appErr != nil {
		t.Fatalf("deprecated existing route was not callable: %v", appErr)
	}
	if appErr := service.ensureOfficialModelSelectable(context.Background(), route); appErr == nil || appErr.Code != "ai.official_model.not_selectable" {
		t.Fatalf("deprecated route selection error=%#v", appErr)
	}
}

func TestCreateRejectsMissingActiveProvider(t *testing.T) {
	service := newTestAgentService(&fakeAIAgentRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 99,
		Name:       "客服助手",
		ModelID:    "gpt-4.1-mini",
		Scenes:     []string{"chat"},
		Status:     enum.CommonYes,
	})

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "AI供应商不存在或已禁用" {
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
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

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

func TestCreateAcceptsAgentGenerateScene(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {
			{ProviderID: 1, ModelID: "gpt-4.1-mini", DisplayName: "GPT-4.1 mini", Status: enum.CommonYes},
		}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "工具生成器",
		ModelID:    "gpt-4.1-mini",
		Scenes:     []string{"agent_generate", "chat", "agent_generate"},
		Status:     enum.CommonYes,
	})

	if appErr != nil {
		t.Fatalf("expected agent_generate scene to be accepted, got %v", appErr)
	}
	if repo.created == nil || repo.created.ScenesJSON != `["agent_generate","chat"]` {
		t.Fatalf("unexpected scenes json: %#v", repo.created)
	}
}

func TestCreateAcceptsCanonicalGenerationScenes(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders: map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {
			{ProviderID: 1, ModelID: "gpt-image-2", DisplayName: "GPT Image 2", Status: enum.CommonYes},
		}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "图片智能体",
		ModelID:    "gpt-image-2",
		Scenes:     []string{"text_generate", "image_generate"},
		Status:     enum.CommonYes,
	})

	if appErr != nil {
		t.Fatalf("expected canonical generation scenes to be accepted, got %#v", appErr)
	}
	if repo.created == nil || repo.created.ScenesJSON != `["text_generate","image_generate"]` {
		t.Fatalf("canonical generation scenes were not persisted: %#v", repo.created)
	}
}

func TestSceneOptionsIncludeAgentAndGenerationScenesOnly(t *testing.T) {
	options := sceneOptions()
	values := map[string]string{}
	for _, item := range options {
		values[item.Value] = item.Label
	}
	expected := map[string]string{
		"chat":           "对话",
		"agent_generate": "工具生成",
		"text_generate":  "文本生成",
		"image_generate": "图片生成",
	}
	if len(options) != len(expected) {
		t.Fatalf("unexpected scene option count: %#v", options)
	}
	for value, label := range expected {
		if values[value] != label {
			t.Fatalf("scene %s label=%q want=%q all=%#v", value, values[value], label, options)
		}
	}
}

func TestCreateRejectsModelOutsideProviderSnapshot(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		ProviderID: 1,
		Name:       "客服助手",
		ModelID:    "gpt-4o",
		Scenes:     []string{"chat"},
		Status:     enum.CommonYes,
	})

	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "关联模型不存在或已禁用" {
		t.Fatalf("expected invalid model error, got %#v", appErr)
	}
}

func TestCreateRejectsInvalidScene(t *testing.T) {
	for _, invalidScene := range []string{"rag", "video_generate", "audio_generate"} {
		t.Run(invalidScene, func(t *testing.T) {
			repo := &fakeAIAgentRepository{
				activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
				modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
			}
			service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

			_, appErr := service.Create(context.Background(), CreateInput{
				ProviderID: 1,
				Name:       "客服助手",
				ModelID:    "gpt-4.1-mini",
				Scenes:     []string{"chat", invalidScene},
				Status:     enum.CommonYes,
			})

			if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "无效的智能体场景" {
				t.Fatalf("expected invalid scene error, got %#v", appErr)
			}
		})
	}
}

func TestListAcceptsCanonicalImageGenerateSceneFilter(t *testing.T) {
	repo := &fakeAIAgentRepository{}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Scene: "image_generate"})

	if appErr != nil {
		t.Fatalf("expected canonical image_generate list scene filter to be accepted, got %#v", appErr)
	}
	if repo.listQuery.Scene != "image_generate" {
		t.Fatalf("repository must receive canonical image_generate scene, got %#v", repo.listQuery)
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
			ProviderName: "OpenAI",
			EngineType:   "openai",
		}},
		total: 1,
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

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

func TestOptionsExcludeDisabledAgents(t *testing.T) {
	repo := &fakeAIAgentRepository{visibleAgents: []AgentWithProvider{
		{Agent: Agent{ID: 1, Name: "启用智能体", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		{Agent: Agent{ID: 2, Name: "禁用智能体", Status: enum.CommonNo, IsDel: enum.CommonNo}},
		{Agent: Agent{ID: 3, Name: "删除智能体", Status: enum.CommonYes, IsDel: enum.CommonYes}},
	}}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	got, appErr := service.Options(context.Background(), OptionQuery{UserID: 9})
	if appErr != nil {
		t.Fatalf("expected options to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].ID != 1 || got.List[0].Name != "启用智能体" {
		t.Fatalf("disabled/deleted agents must be excluded, got %#v", got.List)
	}
	if repo.optionQuery.Scene != "chat" {
		t.Fatalf("blank options scene must default to chat, got %#v", repo.optionQuery)
	}
}

func TestOptionsAcceptsCanonicalImageGenerateSceneFilter(t *testing.T) {
	repo := &fakeAIAgentRepository{visibleAgents: []AgentWithProvider{{Agent: Agent{ID: 7, Name: "图片智能体", Status: enum.CommonYes, IsDel: enum.CommonNo}}}}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	result, appErr := service.Options(context.Background(), OptionQuery{UserID: 9, Scene: "image_generate"})
	if appErr != nil {
		t.Fatalf("expected canonical image_generate scene filter to be accepted, got %#v", appErr)
	}
	if repo.optionQuery.Scene != "image_generate" || len(result.List) != 1 || result.List[0].ID != 7 {
		t.Fatalf("repository must receive canonical image_generate scene, query=%#v result=%#v", repo.optionQuery, result)
	}
}

func TestOptionsExposeOfficialModelAndEffectiveChatCapabilities(t *testing.T) {
	officialID, catalogVersion := "gpt-vision", "official-models-v1"
	mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repo := &fakeAIAgentRepository{
		visibleAgents: []AgentWithProvider{{
			Agent: Agent{ID: 7, ProviderID: 3, Name: "视觉助手", ModelID: "provider-gpt-vision",
				Status: enum.CommonYes, IsDel: enum.CommonNo},
			EngineType: "openai", APIProtocol: aiprovider.APIProtocolResponses, ProviderStatus: enum.CommonYes,
			ProviderModelID: 31, ProviderModelStatus: enum.CommonYes,
			OfficialModelID: officialID, OfficialCatalogVersion: catalogVersion,
			MappingStatus: officialmodel.MappingStatusMapped,
		}, {
			Agent: Agent{ID: 8, ProviderID: 3, Name: "文档助手", ModelID: "provider-gpt-vision",
				Status: enum.CommonYes, IsDel: enum.CommonNo},
			EngineType: "openai", APIProtocol: aiprovider.APIProtocolResponses, ProviderStatus: enum.CommonYes,
			ProviderModelID: 31, ProviderModelStatus: enum.CommonYes,
			OfficialModelID: officialID, OfficialCatalogVersion: catalogVersion,
			MappingStatus: officialmodel.MappingStatusMapped,
		}},
		activeProviders: map[uint64]Provider{3: {
			ID: 3, EngineType: "openai", APIProtocol: aiprovider.APIProtocolResponses, Status: enum.CommonYes, IsDel: enum.CommonNo,
		}},
		modelsByProvider: map[uint64][]ProviderModel{3: {{
			ID: 31, ProviderID: 3, ModelID: "provider-gpt-vision", Status: enum.CommonYes,
			OfficialModelID: &officialID, OfficialCatalogVersion: &catalogVersion,
			MappingStatus: officialmodel.MappingStatusMapped, MappedAt: &mappedAt,
		}}},
	}
	modelResolver := officialmodel.ResolverFunc(func(_ context.Context, requestedModelID string) (officialmodel.ResolvedModel, error) {
		if requestedModelID != "provider-gpt-vision" {
			t.Fatalf("requested model = %q", requestedModelID)
		}
		return officialmodel.ResolvedModel{Model: officialmodel.Model{
			CatalogVersion: catalogVersion, CatalogVendor: "openai", ModelFamily: "gpt",
			ModelID: officialID, LifecycleStatus: officialmodel.LifecycleActive,
			ContextWindowTokens: 128000, MaxOutputTokens: 16384,
			Capabilities: officialmodel.Capabilities{
				InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
				SupportsTools: true, SupportsStreaming: true, SupportsStructuredOutput: true,
				SupportedParameters: []string{"temperature"}, NativeFileInput: true,
				ImageInput: &officialmodel.ImageInputCapability{MIMETypes: []string{"image/png", "image/tiff"}, MaxFiles: 7, MaxBytes: 12 << 20},
			},
		}}, nil
	})
	rules := &countingUploadRuleResolver{rule: uploadpolicy.Rule{MaxFileBytes: 100 << 20, FileExtensions: []string{"pdf", "md", "zip", "go"}}}
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	service := NewService(
		repo,
		box,
		nil,
		WithPricingResolver(modelResolver),
		WithTransportCapabilityResolver(infraai.TransportCapabilityResolverFunc(infraai.DefaultTransportCapabilities)),
		WithUploadRuleResolver(rules),
	)
	result, appErr := service.Options(context.Background(), OptionQuery{UserID: 9})
	if appErr != nil || result == nil || len(result.List) != 2 {
		t.Fatalf("options failed: %#v %#v", result, appErr)
	}
	if rules.calls != 1 {
		t.Fatalf("active upload rule calls=%d", rules.calls)
	}
	option := result.List[0]
	if option.ProviderModelID != 31 || option.OfficialModel == nil || option.OfficialModel.ModelID != officialID ||
		option.OfficialModel.CatalogVersion != catalogVersion {
		t.Fatalf("missing official identity: %#v", option)
	}
	if option.Capabilities == nil || !option.Capabilities.RuntimeParameters.Temperature.Supported ||
		!option.Capabilities.RuntimeParameters.MaxHistory.Supported || !option.Capabilities.RuntimeParameters.MaxHistory.Transitional ||
		option.Capabilities.Attachments.MaxAttachmentsPerMessage != 5 ||
		option.Capabilities.Attachments.MaxMessageAttachmentBytes != 50<<20 ||
		!option.Capabilities.Attachments.Image.Enabled || option.Capabilities.Attachments.Image.MaxFiles != 5 ||
		option.Capabilities.Attachments.Image.MaxFileBytes != 10<<20 ||
		!reflect.DeepEqual(option.Capabilities.Attachments.Image.MIMETypes, []string{"image/png"}) {
		t.Fatalf("unexpected effective capabilities: %#v", option.Capabilities)
	}
	for _, option := range result.List {
		files := option.Capabilities.Attachments.NativeFile
		if !files.Enabled || files.DisabledReason != "" || files.MaxFilesPerMessage != 5 ||
			files.MaxFileBytesExclusive != 50<<20 || files.MaxRequestFileBytes != 50<<20 ||
			!reflect.DeepEqual(files.AcceptedExtensions, []string{"pdf", "md", "go"}) {
			t.Fatalf("native file capability=%#v", files)
		}
	}

	tests := []struct {
		name  string
		rules uploadpolicy.Resolver
	}{
		{name: "resolver missing"},
		{name: "resolver error", rules: uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
			return uploadpolicy.Rule{}, errors.New("upload config unavailable")
		})},
		{name: "empty AI intersection", rules: uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
			return uploadpolicy.Rule{MaxFileBytes: 100 << 20, FileExtensions: []string{"zip", "tar"}}, nil
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(repo, box, nil,
				WithPricingResolver(modelResolver),
				WithTransportCapabilityResolver(infraai.TransportCapabilityResolverFunc(infraai.DefaultTransportCapabilities)),
				WithUploadRuleResolver(test.rules),
			)
			result, appErr := service.Options(context.Background(), OptionQuery{UserID: 9})
			if appErr != nil {
				t.Fatal(appErr)
			}
			for _, option := range result.List {
				attachments := option.Capabilities.Attachments
				if !attachments.Image.Enabled {
					t.Fatal("image capability must remain available")
				}
				if attachments.NativeFile.Enabled || attachments.NativeFile.DisabledReason != capability.NativeFileDisabledPlatform || len(attachments.NativeFile.AcceptedExtensions) != 0 {
					t.Fatalf("native file capability=%#v", attachments.NativeFile)
				}
				rawExtensions, err := json.Marshal(attachments.NativeFile.AcceptedExtensions)
				if err != nil || string(rawExtensions) != "[]" {
					t.Fatalf("disabled accepted extensions JSON=%s err=%v", rawExtensions, err)
				}
			}
		})
	}
}

func TestOptionsRejectsInvalidSceneFilter(t *testing.T) {
	service := newTestAgentService(&fakeAIAgentRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Options(context.Background(), OptionQuery{UserID: 9, Scene: "rag"})
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "无效的智能体场景" {
		t.Fatalf("expected invalid scene error, got %#v", appErr)
	}
}

func TestUpdateOnlyPersistsMVPFields(t *testing.T) {
	repo := &fakeAIAgentRepository{
		rawByID:          map[uint64]Agent{5: {ID: 5, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

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
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAgentRepository{
		rowByID: map[uint64]AgentWithProvider{5: {
			Agent: Agent{ID: 5, ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes, IsDel: enum.CommonNo},
		}},
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", BaseURL: "https://api.openai.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	tester := &fakeAIAgentTester{}
	service := newTestAgentService(repo, box, tester)

	result, appErr := service.Test(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test to succeed, got %v", appErr)
	}
	if result == nil || !result.OK {
		t.Fatalf("expected successful test result, got %#v", result)
	}
	if tester.input.APIKey != "plain-provider-key" || tester.input.BaseURL != "https://api.openai.test/v1" || tester.input.EngineType != infraai.EngineTypeOpenAI {
		t.Fatalf("unexpected tester input: %#v", tester.input)
	}
}

func TestTestReturnsUpstreamError(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAgentRepository{
		rowByID:          map[uint64]AgentWithProvider{5: {Agent: Agent{ID: 5, ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		activeProviders:  map[uint64]Provider{1: {ID: 1, Name: "OpenAI", EngineType: "openai", BaseURL: "https://api.openai.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := newTestAgentService(repo, box, &fakeAIAgentTester{err: errors.New("upstream failed")})

	_, appErr := service.Test(context.Background(), 5)
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || appErr.Message != "测试AI智能体失败" {
		t.Fatalf("expected wrapped upstream error, got %#v", appErr)
	}
}

func TestCreateValidatesBillingMultiplierWithoutPersistingOutputCap(t *testing.T) {
	repo := &fakeAIAgentRepository{
		activeProviders:  map[uint64]Provider{1: {ID: 1, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)
	for _, value := range []string{"0", "-1", "1.1234567"} {
		_, appErr := service.Create(context.Background(), CreateInput{ProviderID: 1, Name: "a", ModelID: "gpt-4.1-mini", BillingMultiplier: value, Status: enum.CommonYes})
		if appErr == nil {
			t.Fatalf("multiplier %q should be rejected", value)
		}
	}
	_, appErr := service.Create(context.Background(), CreateInput{ProviderID: 1, Name: "a", ModelID: "gpt-4.1-mini", BillingMultiplier: "1.25", Status: enum.CommonYes})
	if appErr != nil || repo.created == nil || repo.created.BillingMultiplierPPM != 1250000 {
		t.Fatalf("valid multiplier not persisted: row=%#v err=%v", repo.created, appErr)
	}
}

func TestUpdatePreservesBillingConfigurationWhenProviderChanges(t *testing.T) {
	repo := &fakeAIAgentRepository{
		rawByID:          map[uint64]Agent{5: {ID: 5, BillingMultiplierPPM: 1250000, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeProviders:  map[uint64]Provider{1: {ID: 1, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: enum.CommonYes}}},
	}
	service := newTestAgentService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)
	if appErr := service.Update(context.Background(), 5, UpdateInput{ProviderID: 1, Name: "a", ModelID: "gpt-4.1-mini", Status: enum.CommonYes}); appErr != nil {
		t.Fatalf("update failed: %v", appErr)
	}
	if repo.updates[0]["billing_multiplier_ppm"] != int64(1250000) {
		t.Fatalf("billing fields were reset: %#v", repo.updates[0])
	}
	if _, exists := repo.updates[0]["max_output_tokens"]; exists {
		t.Fatalf("update persisted removed max_output_tokens: %#v", repo.updates[0])
	}
}

func TestPageInitModelOptionsExposeBillingDefaults(t *testing.T) {
	officialID, catalogVersion := "injected-price-model", "catalog-v3"
	mappedAt := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repo := &fakeAIAgentRepository{
		connections:      []Provider{{ID: 1, Name: "OpenAI", EngineType: "openai", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "injected-price-model", OfficialModelID: &officialID, OfficialCatalogVersion: &catalogVersion, MappingStatus: officialmodel.MappingStatusMapped, MappedAt: &mappedAt, Status: enum.CommonYes}}},
	}
	resolverCalls := 0
	resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		resolverCalls++
		if modelID != "injected-price-model" {
			t.Fatalf("resolver model = %q", modelID)
		}
		rates := []pricing.Rate{{Category: pricing.InputTokens, Unit: "token", PriceUnits: 125_000_000, UnitScale: 1_000_000}}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelFamily: "gpt", ModelID: modelID, LifecycleStatus: officialmodel.LifecycleActive, PricingProfile: "standard_global", MaxOutputTokens: 8192},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOverride,
			OverrideVersion: 2, PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})
	result, appErr := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil, WithPricingResolver(resolver)).PageInit(context.Background())
	if appErr != nil || result == nil || len(result.Dict.ModelOptions) != 1 {
		t.Fatalf("page init failed: %#v %v", result, appErr)
	}
	option := result.Dict.ModelOptions[0]
	if resolverCalls != 1 || option.BillingMultiplier != "1" || option.OfficialModel == nil || option.OfficialModel.MaxOutputTokens != 8192 || option.PricingVersion != "catalog-v3:override:2" || option.CatalogVersion != "catalog-v3" || option.PriceSource != "override" || option.OverrideVersion != 2 || len(option.CatalogRates) != 1 || option.CatalogRates[0].Price != "1.25" {
		t.Fatalf("missing billing defaults: %#v", option)
	}
}
