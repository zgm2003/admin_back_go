package aitool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/requestidentity"
	aitext "admin_back_go/internal/module/ai/text"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func TestToolDraftPricingSnapshotUsesInjectedResolver(t *testing.T) {
	resolverCalls := 0
	service := NewService(nil, nil, WithPricingResolver(officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		resolverCalls++
		rates := []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
		}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID, ContextWindowTokens: 8192, MaxOutputTokens: 2048},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
			PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})))
	raw, effective, appErr := service.toolDraftPricingSnapshot(context.Background(), GenerateAgentConfig{
		ModelID: "injected-tool-model", EngineType: "openai", ProviderModelStatus: enum.CommonYes,
		OfficialModelID: "injected-tool-model", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
		BillingMultiplierPPM: 1_000_000,
	})
	if appErr != nil || effective != 2048 || resolverCalls != 1 {
		t.Fatalf("snapshot result = %q, %d, %#v; calls=%d", raw, effective, appErr, resolverCalls)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(raw)
	if err != nil || snapshot.SchemaVersion != aigateway.CurrentPricingSnapshotSchemaVersion || snapshot.RequestedModelID != "injected-tool-model" {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
}

type fakeRepository struct {
	rows             []Tool
	total            int64
	rawByID          map[uint64]Tool
	existingCodes    map[string]uint64
	created          *Tool
	updates          []map[string]any
	statusID         uint64
	status           int
	deletedID        uint64
	boundToolIDs     []uint64
	allActiveToolIDs []uint64
	generateAgents   []GenerateAgentOption
	generateAgent    *GenerateAgentConfig
	replaceAgentID   uint64
	replaceToolIDs   []uint64
	runtimeTools     []RuntimeToolRow
	userCounts       UserCount
	started          *StartToolCallInput
	finished         *FinishToolCallInput
	finishCalls      []FinishToolCallInput
	finishErr        error
	generateLookups  int
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Tool, int64, error) {
	return f.rows, f.total, nil
}
func (f *fakeRepository) GetRaw(ctx context.Context, id uint64) (*Tool, error) {
	if f.rawByID == nil {
		return nil, nil
	}
	row, ok := f.rawByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	if f.existingCodes == nil {
		return false, nil
	}
	id, ok := f.existingCodes[code]
	return ok && id != excludeID, nil
}
func (f *fakeRepository) Create(ctx context.Context, row Tool) (uint64, error) {
	f.created = &row
	return 10, nil
}
func (f *fakeRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}
func (f *fakeRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}
func (f *fakeRepository) Delete(ctx context.Context, id uint64) error { f.deletedID = id; return nil }
func (f *fakeRepository) AgentExists(ctx context.Context, agentID uint64) (bool, error) {
	return agentID == 3 || agentID == 4 || f.replaceAgentID == agentID, nil
}
func (f *fakeRepository) ListGenerateAgents(ctx context.Context) ([]GenerateAgentOption, error) {
	return f.generateAgents, nil
}
func (f *fakeRepository) GetGenerateAgentConfig(ctx context.Context, agentID uint64) (*GenerateAgentConfig, error) {
	f.generateLookups++
	if f.generateAgent == nil || f.generateAgent.AgentID != agentID {
		return nil, nil
	}
	row := *f.generateAgent
	return &row, nil
}
func (f *fakeRepository) ListAllActiveToolIDs(ctx context.Context) ([]uint64, error) {
	return f.allActiveToolIDs, nil
}
func (f *fakeRepository) ListBoundToolIDs(ctx context.Context, agentID uint64) ([]uint64, error) {
	return f.boundToolIDs, nil
}
func (f *fakeRepository) ReplaceAgentTools(ctx context.Context, agentID uint64, toolIDs []uint64) error {
	f.replaceAgentID = agentID
	f.replaceToolIDs = append([]uint64(nil), toolIDs...)
	return nil
}
func (f *fakeRepository) ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeToolRow, error) {
	return f.runtimeTools, nil
}
func (f *fakeRepository) StartToolCall(ctx context.Context, input StartToolCallInput) (uint64, error) {
	f.started = &input
	return 88, nil
}
func (f *fakeRepository) FinishToolCall(ctx context.Context, input FinishToolCallInput) error {
	f.finished = &input
	f.finishCalls = append(f.finishCalls, input)
	return f.finishErr
}
func (f *fakeRepository) CountUsers(ctx context.Context) (UserCount, error) { return f.userCounts, nil }

type fakeDraftTaskService struct {
	inputs     []aitext.AcceptInput
	accepted   map[string]aitext.AcceptInput
	dispatches int
	result     *aitext.Result
	appErr     *apperror.Error
}

func (f *fakeDraftTaskService) ReplayAndWait(_ context.Context, input aitext.ReplayInput) (*aitext.Result, bool, *apperror.Error) {
	accepted, ok := f.accepted[input.RequestID]
	if !ok {
		return nil, false, nil
	}
	fingerprint, err := requestidentity.BuildFingerprint(requestidentity.Input{
		UserID: input.UserID, Operation: input.Operation, Modality: input.Modality, AgentID: int64(input.AgentID),
		ModelID: accepted.ModelID, NormalizedText: input.NormalizedText,
		Options: requestidentity.GenerationOptions{MaxOutputTokens: accepted.EffectiveMaxOutputTokens},
	})
	if err != nil {
		return nil, true, apperror.Wrap(aitext.ErrorCodeRequestInvalid, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "", nil, "request_id无效", err)
	}
	if fingerprint != accepted.RequestFingerprint {
		return nil, true, apperror.Wrap(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "", nil, "request_id冲突", requestidentity.ErrRequestIdentityConflict)
	}
	return f.resultFor(input.RequestID), true, nil
}

func (f *fakeDraftTaskService) SubmitAndWait(_ context.Context, input aitext.AcceptInput) (*aitext.Result, *apperror.Error) {
	f.inputs = append(f.inputs, input)
	if f.appErr != nil {
		return nil, f.appErr
	}
	if f.accepted == nil {
		f.accepted = map[string]aitext.AcceptInput{}
	}
	if stored, ok := f.accepted[input.RequestID]; ok {
		if stored.RequestFingerprint != input.RequestFingerprint {
			return nil, apperror.Wrap(requestidentity.ErrorCodeFingerprintConflict, apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "", nil, "request_id冲突", requestidentity.ErrRequestIdentityConflict)
		}
	} else {
		f.accepted[input.RequestID] = input
		f.dispatches++
	}
	return f.resultFor(input.RequestID), nil
}

func (f *fakeDraftTaskService) resultFor(requestID string) *aitext.Result {
	if f.result != nil {
		copy := *f.result
		return &copy
	}
	return &aitext.Result{TaskID: 41, RunID: 51, RequestID: requestID, Kind: aitext.KindToolDraft, Answer: `{"ok":false,"draft":null,"warnings":[],"clarifying_questions":["请补充需求"]}`}
}

func generateAgentConfig(t *testing.T) GenerateAgentConfig {
	t.Helper()
	return GenerateAgentConfig{
		AgentID: 5, AgentName: "工具生成", ModelID: "gpt-4.1", ModelDisplayName: "GPT-4.1",
		SystemPrompt: "只输出工具草稿JSON", ProviderID: 2, EngineType: "openai", EngineAPIProtocol: infraai.APIProtocolChatCompletions,
		ProviderModelStatus: enum.CommonYes, OfficialModelID: "gpt-4.1", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
		BillingMultiplierPPM: 1_000_000,
	}
}

func TestGenerateDraftRejectsUnknownProviderAPIProtocol(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	agent.EngineAPIProtocol = "legacy"
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{}

	_, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(
		context.Background(),
		GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成工具"},
	)
	if appErr == nil || appErr.Code != aitext.ErrorCodeConfiguration {
		t.Fatalf("unknown provider API protocol error = %#v", appErr)
	}
	if tasks.dispatches != 0 {
		t.Fatalf("invalid provider API protocol dispatched durable work: %d", tasks.dispatches)
	}
}

func testToolPricingResolver() officialmodel.Resolver {
	return officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		rates := []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
		}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID, ContextWindowTokens: 8192, MaxOutputTokens: 2048},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
			PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})
}

func TestCreateRejectsArrayStringOrNullSchemas(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, DefaultExecutors(repo))
	invalidSchemas := []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`"x"`), json.RawMessage(`null`)}
	for _, schema := range invalidSchemas {
		_, appErr := service.Create(context.Background(), MutationInput{
			Name: "查询当前用户量", Code: "admin_user_count", Description: "desc",
			ParametersJSON: schema, ResultSchemaJSON: json.RawMessage(`{"type":"object"}`), RiskLevel: RiskLow, TimeoutMS: 3000, Status: enum.CommonYes,
		})
		if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
			t.Fatalf("schema %s should be rejected, got %#v", string(schema), appErr)
		}
	}
}

func TestCreateStoresToolFieldsExactly(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, DefaultExecutors(repo))
	id, appErr := service.Create(context.Background(), MutationInput{
		Name: " 查询当前用户量 ", Code: " admin_user_count ", Description: " 查询数量 ",
		ParametersJSON:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		ResultSchemaJSON: json.RawMessage(`{"type":"object","properties":{"total_users":{"type":"integer"}}}`),
		RiskLevel:        RiskLow, TimeoutMS: 3000, Status: enum.CommonYes,
	})
	if appErr != nil || id != 10 {
		t.Fatalf("Create returned id=%d err=%v", id, appErr)
	}
	if repo.created == nil || repo.created.Name != "查询当前用户量" || repo.created.Code != "admin_user_count" || repo.created.TimeoutMS != 3000 || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("created row mismatch: %#v", repo.created)
	}
	if !jsonEqualObject(repo.created.ParametersJSON, `{"type":"object","properties":{},"additionalProperties":false}`) {
		t.Fatalf("parameters schema changed: %s", repo.created.ParametersJSON)
	}
}

func TestCreateRejectsEnabledToolWhenCodeHasNoServerImplementation(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, DefaultExecutors(repo))
	_, appErr := service.Create(context.Background(), MutationInput{
		Name: "未知工具", Code: "unknown_tool", Description: "desc",
		ParametersJSON: json.RawMessage(`{"type":"object"}`), ResultSchemaJSON: json.RawMessage(`{"type":"object"}`), RiskLevel: RiskLow, TimeoutMS: 3000, Status: enum.CommonYes,
	})
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("enabled tool with unknown code should be rejected, got %#v", appErr)
	}
}

func TestCreateAllowsDisabledToolBeforeServerImplementationExists(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, DefaultExecutors(repo))
	id, appErr := service.Create(context.Background(), MutationInput{
		Name: "未来工具", Code: "future_tool", Description: "desc",
		ParametersJSON: json.RawMessage(`{"type":"object"}`), ResultSchemaJSON: json.RawMessage(`{"type":"object"}`), RiskLevel: RiskLow, TimeoutMS: 3000, Status: enum.CommonNo,
	})
	if appErr != nil || id != 10 {
		t.Fatalf("disabled future tool should be persisted, id=%d err=%v", id, appErr)
	}
	if repo.created == nil || repo.created.Code != "future_tool" || repo.created.Status != enum.CommonNo {
		t.Fatalf("disabled future tool row mismatch: %#v", repo.created)
	}
}

func TestGeneratePageInitListsAgentGenerateOptions(t *testing.T) {
	repo := &fakeRepository{generateAgents: []GenerateAgentOption{{Label: "工具生成", Value: 5}}}
	got, appErr := NewService(repo, DefaultExecutors(repo)).GeneratePageInit(context.Background())
	if appErr != nil {
		t.Fatalf("GeneratePageInit returned error: %v", appErr)
	}
	if len(got.AgentOptions) != 1 || got.AgentOptions[0].Value != 5 {
		t.Fatalf("unexpected generate options: %#v", got)
	}
}

func TestGenerateDraftRejectsMissingAgentGenerateAgent(t *testing.T) {
	repo := &fakeRepository{}
	_, appErr := NewService(repo, DefaultExecutors(repo)).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 99, UserID: 7, Requirement: "生成查询用户数工具"})
	if appErr == nil || appErr.LegacyCode != apperror.CodeNotFound {
		t.Fatalf("missing generate agent should be rejected, got %#v", appErr)
	}
}

func TestGenerateDraftRejectsBlankRequirement(t *testing.T) {
	repo := &fakeRepository{}
	_, appErr := NewService(repo, DefaultExecutors(repo)).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "   "})
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("blank requirement should be rejected, got %#v", appErr)
	}
}

func TestGenerateDraftRejectsBlankRequestIDBeforeAgentLookup(t *testing.T) {
	repo := &fakeRepository{}
	_, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(&fakeDraftTaskService{})).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "生成工具"})
	if appErr == nil || appErr.Code != aitext.ErrorCodeRequestInvalid {
		t.Fatalf("blank request_id should be rejected, got %#v", appErr)
	}
	if repo.generateLookups != 0 {
		t.Fatalf("agent loaded before request_id validation: %d", repo.generateLookups)
	}
}

func TestGenerateDraftParsesStrictJSONDraft(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{result: &aitext.Result{TaskID: 41, RunID: 51, RequestID: "request-1", Kind: aitext.KindToolDraft, Answer: `{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回后台用户数量统计，不返回个人信息。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`, PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成查询当前用户量工具", CodeHint: "admin_user_count"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if !got.OK || got.Draft == nil || got.Draft.Code != "admin_user_count" || got.Draft.Status != enum.CommonYes {
		t.Fatalf("unexpected draft: %#v", got)
	}
	if got.Usage == nil || got.Usage.TotalTokens != 18 {
		t.Fatalf("usage should be returned from durable run: %#v", got.Usage)
	}
	if len(tasks.inputs) != 1 || tasks.inputs[0].Kind != aitext.KindToolDraft {
		t.Fatalf("durable task input mismatch: %#v", tasks.inputs)
	}
}

func TestNormalizeGenerateDraftCandidateRejectsInvalidDraftBeforeSettlement(t *testing.T) {
	for _, raw := range []string{
		`not-json`,
		`{"ok":true,"draft":{"name":"broken","code":"BAD CODE","description":"invalid","parameters_json":{},"result_schema_json":{},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`,
	} {
		if normalized, appErr := NormalizeGenerateDraftCandidate(raw); appErr == nil || normalized != "" {
			t.Fatalf("invalid candidate normalized=%q error=%#v", normalized, appErr)
		}
	}

	raw := `{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回数量统计。","parameters_json":{"type":"object","properties":{},"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`
	normalized, appErr := NormalizeGenerateDraftCandidate(raw)
	if appErr != nil || normalized == "" {
		t.Fatalf("valid candidate normalized=%q error=%v", normalized, appErr)
	}
	if !strings.Contains(normalized, `"required":[]`) {
		t.Fatalf("candidate was not normalized before settlement: %s", normalized)
	}
}

func TestGenerateDraftNormalizesSchemaWithoutRequired(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{result: &aitext.Result{Answer: `{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回数量统计。","parameters_json":{"type":"object","properties":{},"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`}}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成查询当前用户量工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft should accept JSON Schema without required: %v", appErr)
	}
	if got.Draft == nil || !jsonEqualObject(string(got.Draft.ParametersJSON), `{"type":"object","properties":{},"required":[],"additionalProperties":false}`) {
		t.Fatalf("parameters schema should be normalized with empty required: %#v", got.Draft)
	}
}

func TestGenerateDraftReturnsClarifyingQuestionsWhenModelSaysNotEnough(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{result: &aitext.Result{Answer: `{"ok":false,"draft":null,"warnings":["需求不足，暂不生成工具草稿"],"clarifying_questions":["请说明入参和返回字段？"]}`}}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "做个工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.OK || got.Draft != nil || len(got.ClarifyingQuestions) != 1 {
		t.Fatalf("expected clarifying response, got %#v", got)
	}
}

func TestGenerateDraftForcesDisabledWhenExecutorMissing(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{result: &aitext.Result{Answer: `{"ok":true,"draft":{"name":"未来工具","code":"future_tool","description":"未来服务端实现后才能启用。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`}}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成未来工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.Draft == nil || got.Draft.Status != enum.CommonNo || len(got.Warnings) != 1 || got.Warnings[0] != unregisteredToolWarning {
		t.Fatalf("unregistered generated tool should be disabled with warning: %#v", got)
	}
}

func TestGenerateDraftCanReturnEnabledWhenExecutorRegistered(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{result: &aitext.Result{Answer: `{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回数量统计。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`}}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver())).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成已实现工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.Draft == nil || got.Draft.Status != enum.CommonYes || len(got.Warnings) != 0 {
		t.Fatalf("registered generated tool should stay enabled: %#v", got)
	}
}

func TestGenerateDraftNormalizesFingerprintAndConflictsBeforeSecondDispatch(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{}
	service := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver()))

	_, firstErr := service.GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-replay", AgentID: 5, UserID: 7, Requirement: "  查询用户数\r\n", CodeHint: " admin_user_count "})
	_, replayErr := service.GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-replay", AgentID: 5, UserID: 7, Requirement: "查询用户数\n", CodeHint: "admin_user_count"})
	if firstErr != nil || replayErr != nil {
		t.Fatalf("normalized replay errors: first=%v replay=%v", firstErr, replayErr)
	}
	_, conflictErr := service.GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-replay", AgentID: 5, UserID: 7, Requirement: "查询订单数", CodeHint: "admin_user_count"})
	if conflictErr == nil || conflictErr.Code != requestidentity.ErrorCodeFingerprintConflict || conflictErr.HTTPStatus != http.StatusConflict {
		t.Fatalf("different fingerprint error = %#v", conflictErr)
	}
	if tasks.dispatches != 1 {
		t.Fatalf("provider-equivalent durable dispatches = %d, want 1", tasks.dispatches)
	}
}

func TestGenerateDraftReplaysPersistedResultAfterAgentBecomesUnavailable(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{}
	service := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver()))
	input := GenerateDraftInput{RequestID: "request-durable-replay", AgentID: 5, UserID: 7, Requirement: "查询用户数", CodeHint: "admin_user_count"}

	first, firstErr := service.GenerateDraft(context.Background(), input)
	if firstErr != nil || first == nil {
		t.Fatalf("first result=%#v error=%v", first, firstErr)
	}
	repo.generateAgent = nil
	replay, replayErr := service.GenerateDraft(context.Background(), input)

	if replayErr != nil || replay == nil {
		t.Fatalf("durable replay result=%#v error=%v", replay, replayErr)
	}
	if repo.generateLookups != 1 {
		t.Fatalf("durable replay reloaded mutable agent: lookups=%d", repo.generateLookups)
	}
	if tasks.dispatches != 1 {
		t.Fatalf("durable replay dispatched again: %d", tasks.dispatches)
	}
}

func TestGenerateDraftRejectsPersistedFingerprintConflictBeforeUnavailableAgentLookup(t *testing.T) {
	repo := &fakeRepository{}
	agent := generateAgentConfig(t)
	repo.generateAgent = &agent
	tasks := &fakeDraftTaskService{}
	service := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(testToolPricingResolver()))

	_, firstErr := service.GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-durable-conflict", AgentID: 5, UserID: 7, Requirement: "查询用户数"})
	if firstErr != nil {
		t.Fatalf("first error=%v", firstErr)
	}
	repo.generateAgent = nil
	_, conflictErr := service.GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-durable-conflict", AgentID: 5, UserID: 7, Requirement: "查询订单数"})

	if conflictErr == nil || conflictErr.Code != requestidentity.ErrorCodeFingerprintConflict || conflictErr.HTTPStatus != http.StatusConflict {
		t.Fatalf("persisted conflict error=%#v", conflictErr)
	}
	if repo.generateLookups != 1 {
		t.Fatalf("conflict reached mutable agent lookup: %d", repo.generateLookups)
	}
	if tasks.dispatches != 1 {
		t.Fatalf("conflict dispatched new work: %d", tasks.dispatches)
	}
}

func TestGenerateDraftUsesDistinctStablePriceAndUpperBoundCodes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*GenerateAgentConfig)
		code   string
	}{
		{name: "missing price", mutate: func(agent *GenerateAgentConfig) {
			agent.ModelID, agent.OfficialModelID = "private-unpriced-model", "private-unpriced-model"
		}, code: aitext.ErrorCodePriceUnavailable},
		{name: "unsafe official output cap", mutate: func(agent *GenerateAgentConfig) {
			agent.ModelID, agent.OfficialModelID = "unsafe-official-cap", "unsafe-official-cap"
		}, code: aitext.ErrorCodeUnsafeUpperBound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			agent := generateAgentConfig(t)
			tc.mutate(&agent)
			repo.generateAgent = &agent
			tasks := &fakeDraftTaskService{}
			resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
				if modelID == "private-unpriced-model" {
					return officialmodel.ResolvedModel{}, officialmodel.ErrPriceUnavailable
				}
				rates := []pricing.Rate{
					{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
					{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
				}
				maxOutputTokens := int64(2048)
				if modelID == "unsafe-official-cap" {
					maxOutputTokens = 0
				}
				return officialmodel.ResolvedModel{
					Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID, ContextWindowTokens: maxOutputTokens * 4, MaxOutputTokens: maxOutputTokens},
					EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
					PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
				}, nil
			})
			_, appErr := NewService(repo, DefaultExecutors(repo), WithDraftTaskService(tasks), WithPricingResolver(resolver)).GenerateDraft(context.Background(), GenerateDraftInput{RequestID: "request-1", AgentID: 5, UserID: 7, Requirement: "生成工具"})
			if appErr == nil || appErr.Code != tc.code {
				t.Fatalf("error = %#v, want %s", appErr, tc.code)
			}
			if tasks.dispatches != 0 {
				t.Fatalf("invalid pricing dispatched durable worker: %d", tasks.dispatches)
			}
		})
	}
}

func TestChangeStatusRejectsEnableWhenCodeHasNoServerImplementation(t *testing.T) {
	repo := &fakeRepository{rawByID: map[uint64]Tool{
		7: {ID: 7, Name: "未来工具", Code: "future_tool", Status: enum.CommonNo},
	}}
	service := NewService(repo, DefaultExecutors(repo))
	appErr := service.ChangeStatus(context.Background(), 7, enum.CommonYes)
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("enable should be rejected when code has no server implementation, got %#v", appErr)
	}
	if repo.statusID != 0 {
		t.Fatalf("status changed despite missing server implementation: id=%d status=%d", repo.statusID, repo.status)
	}
}

func TestUpdateAgentToolsReplacesBindings(t *testing.T) {
	repo := &fakeRepository{
		allActiveToolIDs: []uint64{1, 2, 3},
		rawByID: map[uint64]Tool{
			1: {ID: 1, Code: "admin_user_count", RiskLevel: RiskLow, Status: enum.CommonYes},
			3: {ID: 3, Code: "admin_user_count", RiskLevel: RiskLow, Status: enum.CommonYes},
		},
	}
	service := NewService(repo, DefaultExecutors(repo))
	appErr := service.UpdateAgentTools(context.Background(), 3, UpdateAgentToolsInput{ToolIDs: []uint64{3, 1, 1}})
	if appErr != nil {
		t.Fatalf("UpdateAgentTools returned error: %v", appErr)
	}
	if repo.replaceAgentID != 3 || len(repo.replaceToolIDs) != 2 || repo.replaceToolIDs[0] != 1 || repo.replaceToolIDs[1] != 3 {
		t.Fatalf("bindings not normalized/replaced: agent=%d tools=%#v", repo.replaceAgentID, repo.replaceToolIDs)
	}
}

func TestListRuntimeToolsReturnsOnlyLowRiskRegisteredValidTools(t *testing.T) {
	validSchema := `{"type":"object","properties":{},"additionalProperties":false}`
	t.Run("filters unavailable tools", func(t *testing.T) {
		repo := &fakeRepository{runtimeTools: []RuntimeToolRow{
			{ToolID: 1, Name: "禁用绑定", Code: "admin_user_count", ParametersJSON: validSchema, ResultSchemaJSON: validSchema, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonNo},
			{ToolID: 2, Name: "禁用工具", Code: "admin_user_count", ParametersJSON: validSchema, ResultSchemaJSON: validSchema, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonNo, BindingStatus: enum.CommonYes},
			{ToolID: 3, Name: "中风险", Code: "admin_user_count", ParametersJSON: validSchema, ResultSchemaJSON: validSchema, RiskLevel: RiskMedium, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonYes},
			{ToolID: 4, Name: "未注册", Code: "future_tool", ParametersJSON: validSchema, ResultSchemaJSON: validSchema, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonYes},
			{ToolID: 5, Name: "查询用户量", Code: "admin_user_count", ParametersJSON: validSchema, ResultSchemaJSON: validSchema, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonYes},
		}}
		tools, appErr := NewService(repo, DefaultExecutors(repo)).ListRuntimeTools(context.Background(), 3)
		if appErr != nil {
			t.Fatalf("ListRuntimeTools returned error: %v", appErr)
		}
		if len(tools) != 1 || tools[0].ID != 5 || tools[0].Code != "admin_user_count" {
			t.Fatalf("runtime tools not filtered: %#v", tools)
		}
	})

	for _, test := range []struct {
		name       string
		parameters string
		result     string
	}{
		{name: "invalid parameters schema", parameters: "{", result: validSchema},
		{name: "invalid result schema", parameters: validSchema, result: "[]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{runtimeTools: []RuntimeToolRow{{
				ToolID: 5, Name: "查询用户量", Code: "admin_user_count",
				ParametersJSON: test.parameters, ResultSchemaJSON: test.result,
				RiskLevel: RiskLow, TimeoutMS: 3000,
				ToolStatus: enum.CommonYes, BindingStatus: enum.CommonYes,
			}}}
			tools, appErr := NewService(repo, DefaultExecutors(repo)).ListRuntimeTools(context.Background(), 3)
			if tools != nil || appErr == nil || appErr.HTTPStatus != 500 {
				t.Fatalf("tools=%#v err=%#v", tools, appErr)
			}
		})
	}
}

func TestUpdateAgentToolsRejectsNonLowRiskOrUnregisteredTools(t *testing.T) {
	for _, tool := range []Tool{
		{ID: 7, Code: "admin_user_count", RiskLevel: RiskHigh, Status: enum.CommonYes},
		{ID: 8, Code: "future_tool", RiskLevel: RiskLow, Status: enum.CommonYes},
	} {
		t.Run(tool.Code+"_"+tool.RiskLevel, func(t *testing.T) {
			repo := &fakeRepository{
				allActiveToolIDs: []uint64{tool.ID},
				rawByID:          map[uint64]Tool{tool.ID: tool},
			}
			appErr := NewService(repo, DefaultExecutors(repo)).UpdateAgentTools(context.Background(), 3, UpdateAgentToolsInput{ToolIDs: []uint64{tool.ID}})
			if appErr == nil || appErr.HTTPStatus != 400 || repo.replaceAgentID != 0 {
				t.Fatalf("error=%#v replacement=%d", appErr, repo.replaceAgentID)
			}
		})
	}
}

type recordingExecutor struct {
	calls          int
	result         map[string]any
	err            error
	waitForContext bool
}

func (e *recordingExecutor) Execute(ctx context.Context, _ json.RawMessage) (map[string]any, error) {
	e.calls++
	if e.waitForContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return e.result, e.err
}

func executableRuntimeTool() RuntimeTool {
	return RuntimeTool{
		ID: 5, Name: "查询用户量", Code: "admin_user_count", RiskLevel: RiskLow, TimeoutMS: 100,
		ParametersJSON: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope": map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		},
		ResultSchemaJSON: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"total_users": map[string]any{"type": "integer"},
			},
			"required":             []any{"total_users"},
			"additionalProperties": false,
		},
	}
}

func TestExecuteRejectsInvalidJSONBeforeExecutorAndAuditsFailure(t *testing.T) {
	repo := &fakeRepository{}
	executor := &recordingExecutor{result: map[string]any{"total_users": 1}}
	raw := json.RawMessage(`{"secret":"oops"`)

	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: executableRuntimeTool(), CallID: "call-1", Arguments: raw,
	})

	if result != nil || appErr == nil || executor.calls != 0 || repo.started == nil {
		t.Fatalf("result=%#v err=%#v calls=%d started=%#v", result, appErr, executor.calls, repo.started)
	}
	var audit map[string]any
	if err := json.Unmarshal(repo.started.ArgumentsJSON, &audit); err != nil {
		t.Fatalf("audit arguments are not JSON: %s", repo.started.ArgumentsJSON)
	}
	if audit["invalid_json"] != true || audit["byte_length"] != float64(len(raw)) || len(audit["sha256"].(string)) != 64 || strings.Contains(string(repo.started.ArgumentsJSON), "secret") {
		t.Fatalf("unsafe invalid JSON audit envelope: %s", repo.started.ArgumentsJSON)
	}
	if len(repo.finishCalls) != 1 || repo.finishCalls[0].Status != ToolCallFailed {
		t.Fatalf("finish calls=%#v", repo.finishCalls)
	}
}

func TestExecuteRejectsArgumentsOutsideSchema(t *testing.T) {
	repo := &fakeRepository{}
	executor := &recordingExecutor{result: map[string]any{"total_users": 1}}
	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: executableRuntimeTool(), Arguments: json.RawMessage(`{"scope":7}`),
	})
	if result != nil || appErr == nil || executor.calls != 0 || len(repo.finishCalls) != 1 || repo.finishCalls[0].Status != ToolCallFailed {
		t.Fatalf("result=%#v err=%#v calls=%d finishes=%#v", result, appErr, executor.calls, repo.finishCalls)
	}
}

func TestExecuteRejectsResultOutsideSchema(t *testing.T) {
	repo := &fakeRepository{}
	executor := &recordingExecutor{result: map[string]any{"total_users": "one"}}
	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: executableRuntimeTool(), Arguments: json.RawMessage(`{"scope":"all"}`),
	})
	if result != nil || appErr == nil || executor.calls != 1 || len(repo.finishCalls) != 1 || repo.finishCalls[0].Status != ToolCallFailed {
		t.Fatalf("result=%#v err=%#v calls=%d finishes=%#v", result, appErr, executor.calls, repo.finishCalls)
	}
}

func TestExecuteMarksTimeoutAndNeverReportsSuccess(t *testing.T) {
	repo := &fakeRepository{}
	executor := &recordingExecutor{waitForContext: true}
	tool := executableRuntimeTool()
	tool.TimeoutMS = 1
	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: tool, Arguments: json.RawMessage(`{"scope":"all"}`),
	})
	if result != nil || appErr == nil || executor.calls != 1 || len(repo.finishCalls) != 1 || repo.finishCalls[0].Status != ToolCallTimeout {
		t.Fatalf("result=%#v err=%#v calls=%d finishes=%#v", result, appErr, executor.calls, repo.finishCalls)
	}
}

func TestExecuteRejectsNonLowRiskToolAtRuntime(t *testing.T) {
	repo := &fakeRepository{}
	executor := &recordingExecutor{result: map[string]any{"total_users": 1}}
	tool := executableRuntimeTool()
	tool.RiskLevel = RiskMedium
	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: tool, Arguments: json.RawMessage(`{"scope":"all"}`),
	})
	if result != nil || appErr == nil || executor.calls != 0 || len(repo.finishCalls) != 1 || repo.finishCalls[0].Status != ToolCallFailed {
		t.Fatalf("result=%#v err=%#v calls=%d finishes=%#v", result, appErr, executor.calls, repo.finishCalls)
	}
}

func TestExecuteReturnsAuditFinalizationFailure(t *testing.T) {
	repo := &fakeRepository{finishErr: errors.New("audit unavailable")}
	executor := &recordingExecutor{result: map[string]any{"total_users": 1}}
	result, appErr := NewService(repo, map[string]Executor{"admin_user_count": executor}).Execute(context.Background(), ExecuteInput{
		RunID: 9, Tool: executableRuntimeTool(), Arguments: json.RawMessage(`{"scope":"all"}`),
	})
	if result != nil || appErr == nil || !strings.Contains(appErr.Error(), "更新AI工具调用记录失败") {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
}

func TestAdminUserCountReturnsCountsAndNoPersonalFields(t *testing.T) {
	repo := &fakeRepository{userCounts: UserCount{TotalUsers: 1015, EnabledUsers: 1015, DisabledUsers: 0}}
	result, err := NewAdminUserCountExecutor(repo).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	raw, _ := json.Marshal(result)
	body := string(raw)
	if body != `{"disabled_users":0,"enabled_users":1015,"total_users":1015}` {
		t.Fatalf("unexpected result: %s", body)
	}
	for _, forbidden := range []string{"username", "email", "phone", "password", "list"} {
		if jsonContainsKey(body, forbidden) {
			t.Fatalf("tool result leaked personal/list field %q in %s", forbidden, body)
		}
	}
}

func jsonContainsKey(raw string, key string) bool {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return false
	}
	_, ok := value[key]
	return ok
}

func jsonEqualObject(a string, b string) bool {
	var left map[string]any
	var right map[string]any
	if err := json.Unmarshal([]byte(a), &left); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &right); err != nil {
		return false
	}
	return left["type"] == right["type"] && len(left) == len(right)
}
