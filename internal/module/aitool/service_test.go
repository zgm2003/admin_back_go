package aitool

import (
	"context"
	"encoding/json"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

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
	return nil
}
func (f *fakeRepository) CountUsers(ctx context.Context) (UserCount, error) { return f.userCounts, nil }

type fakeGenerateEngineFactory struct {
	input  EngineConfig
	engine platformai.Engine
}

func (f *fakeGenerateEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	f.input = input
	if f.engine != nil {
		return f.engine, nil
	}
	return platformai.NewFakeEngine(`{"ok":false,"draft":null,"warnings":[],"clarifying_questions":["请补充需求"]}`), nil
}

func generateAgentConfig(t *testing.T, box secretbox.Box) GenerateAgentConfig {
	t.Helper()
	cipher, err := box.Encrypt("plain-provider-key")
	if err != nil {
		t.Fatalf("encrypt test api key: %v", err)
	}
	return GenerateAgentConfig{
		AgentID:         5,
		AgentName:       "工具生成",
		ModelID:         "gpt-test",
		SystemPrompt:    "只输出工具草稿JSON",
		ProviderID:      2,
		EngineType:      string(platformai.EngineTypeOpenAI),
		EngineBaseURL:   "https://api.openai.test/v1",
		EngineAPIKeyEnc: cipher,
	}
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
		if appErr == nil || appErr.Code != apperror.CodeBadRequest {
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
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
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
	_, appErr := NewService(repo, DefaultExecutors(repo)).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 99, UserID: 7, Requirement: "生成查询用户数工具"})
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("missing generate agent should be rejected, got %#v", appErr)
	}
}

func TestGenerateDraftRejectsBlankRequirement(t *testing.T) {
	repo := &fakeRepository{}
	_, appErr := NewService(repo, DefaultExecutors(repo)).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "   "})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("blank requirement should be rejected, got %#v", appErr)
	}
}

func TestGenerateDraftParsesStrictJSONDraft(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	agent := generateAgentConfig(t, box)
	repo.generateAgent = &agent
	engine := platformai.NewFakeEngine(`{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回后台用户数量统计，不返回个人信息。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`)
	factory := &fakeGenerateEngineFactory{engine: engine}
	got, appErr := NewService(repo, DefaultExecutors(repo), WithSecretbox(box), WithEngineFactory(factory)).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "生成查询当前用户量工具", CodeHint: "admin_user_count"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if !got.OK || got.Draft == nil || got.Draft.Code != "admin_user_count" || got.Draft.Status != enum.CommonYes {
		t.Fatalf("unexpected draft: %#v", got)
	}
	if factory.input.APIKey != "plain-provider-key" || factory.input.EngineType != platformai.EngineTypeOpenAI {
		t.Fatalf("engine config not built from active provider: %#v", factory.input)
	}
	if got.Usage == nil || got.Usage.TotalTokens != 2 {
		t.Fatalf("usage should be returned from engine result: %#v", got.Usage)
	}
}

func TestGenerateDraftNormalizesSchemaWithoutRequired(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	agent := generateAgentConfig(t, box)
	repo.generateAgent = &agent
	engine := platformai.NewFakeEngine(`{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回数量统计。","parameters_json":{"type":"object","properties":{},"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`)
	got, appErr := NewService(repo, DefaultExecutors(repo), WithSecretbox(box), WithEngineFactory(&fakeGenerateEngineFactory{engine: engine})).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "生成查询当前用户量工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft should accept JSON Schema without required: %v", appErr)
	}
	if got.Draft == nil || !jsonEqualObject(string(got.Draft.ParametersJSON), `{"type":"object","properties":{},"required":[],"additionalProperties":false}`) {
		t.Fatalf("parameters schema should be normalized with empty required: %#v", got.Draft)
	}
}

func TestGenerateDraftReturnsClarifyingQuestionsWhenModelSaysNotEnough(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	agent := generateAgentConfig(t, box)
	repo.generateAgent = &agent
	engine := platformai.NewFakeEngine(`{"ok":false,"draft":null,"warnings":["需求不足，暂不生成工具草稿"],"clarifying_questions":["请说明入参和返回字段？"]}`)
	got, appErr := NewService(repo, DefaultExecutors(repo), WithSecretbox(box), WithEngineFactory(&fakeGenerateEngineFactory{engine: engine})).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "做个工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.OK || got.Draft != nil || len(got.ClarifyingQuestions) != 1 {
		t.Fatalf("expected clarifying response, got %#v", got)
	}
}

func TestGenerateDraftForcesDisabledWhenExecutorMissing(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	agent := generateAgentConfig(t, box)
	repo.generateAgent = &agent
	engine := platformai.NewFakeEngine(`{"ok":true,"draft":{"name":"未来工具","code":"future_tool","description":"未来服务端实现后才能启用。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`)
	got, appErr := NewService(repo, DefaultExecutors(repo), WithSecretbox(box), WithEngineFactory(&fakeGenerateEngineFactory{engine: engine})).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "生成未来工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.Draft == nil || got.Draft.Status != enum.CommonNo || len(got.Warnings) != 1 || got.Warnings[0] != unregisteredToolWarning {
		t.Fatalf("unregistered generated tool should be disabled with warning: %#v", got)
	}
}

func TestGenerateDraftCanReturnEnabledWhenExecutorRegistered(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	agent := generateAgentConfig(t, box)
	repo.generateAgent = &agent
	engine := platformai.NewFakeEngine(`{"ok":true,"draft":{"name":"查询当前用户量","code":"admin_user_count","description":"只返回数量统计。","parameters_json":{"type":"object","properties":{},"required":[],"additionalProperties":false},"result_schema_json":{"type":"object","properties":{"total_users":{"type":"integer"}},"required":["total_users"],"additionalProperties":false},"risk_level":"low","timeout_ms":3000,"status":1},"warnings":[],"clarifying_questions":[]}`)
	got, appErr := NewService(repo, DefaultExecutors(repo), WithSecretbox(box), WithEngineFactory(&fakeGenerateEngineFactory{engine: engine})).GenerateDraft(context.Background(), GenerateDraftInput{AgentID: 5, UserID: 7, Requirement: "生成已实现工具"})
	if appErr != nil {
		t.Fatalf("GenerateDraft returned error: %v", appErr)
	}
	if got.Draft == nil || got.Draft.Status != enum.CommonYes || len(got.Warnings) != 0 {
		t.Fatalf("registered generated tool should stay enabled: %#v", got)
	}
}

func TestChangeStatusRejectsEnableWhenCodeHasNoServerImplementation(t *testing.T) {
	repo := &fakeRepository{rawByID: map[uint64]Tool{
		7: {ID: 7, Name: "未来工具", Code: "future_tool", Status: enum.CommonNo},
	}}
	service := NewService(repo, DefaultExecutors(repo))
	appErr := service.ChangeStatus(context.Background(), 7, enum.CommonYes)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("enable should be rejected when code has no server implementation, got %#v", appErr)
	}
	if repo.statusID != 0 {
		t.Fatalf("status changed despite missing server implementation: id=%d status=%d", repo.statusID, repo.status)
	}
}

func TestUpdateAgentToolsReplacesBindings(t *testing.T) {
	repo := &fakeRepository{allActiveToolIDs: []uint64{1, 2, 3}}
	service := NewService(repo, DefaultExecutors(repo))
	appErr := service.UpdateAgentTools(context.Background(), 3, UpdateAgentToolsInput{ToolIDs: []uint64{3, 1, 1}})
	if appErr != nil {
		t.Fatalf("UpdateAgentTools returned error: %v", appErr)
	}
	if repo.replaceAgentID != 3 || len(repo.replaceToolIDs) != 2 || repo.replaceToolIDs[0] != 1 || repo.replaceToolIDs[1] != 3 {
		t.Fatalf("bindings not normalized/replaced: agent=%d tools=%#v", repo.replaceAgentID, repo.replaceToolIDs)
	}
}

func TestListRuntimeToolsFiltersDisabledBindingsAndTools(t *testing.T) {
	repo := &fakeRepository{runtimeTools: []RuntimeToolRow{
		{ToolID: 1, Name: "启用", Code: "enabled", ParametersJSON: `{"type":"object"}`, ResultSchemaJSON: `{"type":"object"}`, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonYes},
		{ToolID: 2, Name: "禁用绑定", Code: "binding_disabled", ParametersJSON: `{"type":"object"}`, ResultSchemaJSON: `{"type":"object"}`, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonYes, BindingStatus: enum.CommonNo},
		{ToolID: 3, Name: "禁用工具", Code: "tool_disabled", ParametersJSON: `{"type":"object"}`, ResultSchemaJSON: `{"type":"object"}`, RiskLevel: RiskLow, TimeoutMS: 3000, ToolStatus: enum.CommonNo, BindingStatus: enum.CommonYes},
	}}
	tools, appErr := NewService(repo, DefaultExecutors(repo)).ListRuntimeTools(context.Background(), 3)
	if appErr != nil {
		t.Fatalf("ListRuntimeTools returned error: %v", appErr)
	}
	if len(tools) != 1 || tools[0].Code != "enabled" || tools[0].ParametersJSON["type"] != "object" {
		t.Fatalf("runtime tools not filtered/mapped: %#v", tools)
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
