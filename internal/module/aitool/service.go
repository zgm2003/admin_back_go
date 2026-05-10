package aitool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"
const sceneAgentGenerate = "agent_generate"
const unregisteredToolWarning = "该工具编码暂未注册服务端实现，已默认禁用"

type Service struct {
	repository    Repository
	executors     map[string]Executor
	secretbox     secretbox.Box
	engineFactory EngineFactory
	now           func() time.Time
}

type Option func(*Service)

func WithSecretbox(box secretbox.Box) Option {
	return func(s *Service) {
		s.secretbox = box
	}
}

func WithEngineFactory(factory EngineFactory) Option {
	return func(s *Service) {
		s.engineFactory = factory
	}
}

func NewService(repository Repository, executors map[string]Executor, opts ...Option) *Service {
	service := &Service{repository: repository, executors: executors, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{RiskLevelArr: riskOptions(), CommonStatusArr: dict.CommonStatusOptions()}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	list := make([]ToolDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, toolDTO(row))
	}
	return &ListResponse{List: list, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: total, TotalPage: totalPage(total, query.PageSize)}}, nil
}

func (s *Service) GeneratePageInit(ctx context.Context) (*GeneratePageInitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	options, err := repo.ListGenerateAgents(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI生成智能体失败", err)
	}
	if options == nil {
		options = []GenerateAgentOption{}
	}
	return &GeneratePageInitResponse{AgentOptions: options}, nil
}

func (s *Service) GenerateDraft(ctx context.Context, input GenerateDraftInput) (*GenerateDraftResponse, *apperror.Error) {
	if input.AgentID == 0 {
		return nil, apperror.BadRequest("AI生成智能体不能为空")
	}
	if input.UserID == 0 {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	requirement := strings.TrimSpace(input.Requirement)
	if requirement == "" {
		return nil, apperror.BadRequest("工具需求描述不能为空")
	}
	if len([]rune(requirement)) > 4000 {
		return nil, apperror.BadRequest("工具需求描述不能超过4000个字符")
	}
	codeHint := strings.TrimSpace(input.CodeHint)
	if len([]rune(codeHint)) > 64 {
		return nil, apperror.BadRequest("工具编码提示不能超过64个字符")
	}
	if codeHint != "" && !validToolCode(codeHint) {
		return nil, apperror.BadRequest("工具编码提示只能使用小写字母、数字、下划线，长度3到64")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	agent, err := repo.GetGenerateAgentConfig(ctx, input.AgentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI生成智能体失败", err)
	}
	if agent == nil {
		return nil, apperror.NotFound("AI生成智能体不存在或未启用")
	}
	if strings.TrimSpace(agent.SystemPrompt) == "" {
		return nil, apperror.BadRequest("AI生成智能体系统提示词未配置")
	}
	engine, appErr := s.engineForGenerateAgent(ctx, *agent)
	if appErr != nil {
		return nil, appErr
	}
	result, err := engine.StreamChat(ctx, platformai.ChatInput{
		AgentID: input.AgentID,
		UserID:  input.UserID,
		UserKey: fmt.Sprintf("admin:%d", input.UserID),
		Content: buildToolGenerateUserPrompt(requirement, codeHint),
		Inputs: map[string]any{
			"model_id":      agent.ModelID,
			"system_prompt": agent.SystemPrompt,
		},
	}, discardEventSink{})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "AI生成工具草稿失败", err)
	}
	if result == nil || strings.TrimSpace(result.Answer) == "" {
		return nil, apperror.Internal("AI生成工具草稿为空")
	}
	response, appErr := decodeGenerateDraftResponse(result.Answer)
	if appErr != nil {
		return nil, appErr
	}
	response.Usage = generateUsage(result)
	if !response.OK {
		response.Draft = nil
		response.Warnings = trimStringList(response.Warnings)
		response.ClarifyingQuestions = trimStringList(response.ClarifyingQuestions)
		return response, nil
	}
	draft, appErr := s.normalizeGeneratedDraft(response.Draft)
	if appErr != nil {
		return nil, appErr
	}
	response.Draft = draft
	response.Warnings = trimStringList(response.Warnings)
	response.ClarifyingQuestions = trimStringList(response.ClarifyingQuestions)
	if !s.executorRegistered(draft.Code) {
		response.Draft.Status = enum.CommonNo
		if !containsString(response.Warnings, unregisteredToolWarning) {
			response.Warnings = append(response.Warnings, unregisteredToolWarning)
		}
	}
	return response, nil
}

func (s *Service) Create(ctx context.Context, input MutationInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeMutation(input)
	if appErr != nil {
		return 0, appErr
	}
	if row.Status == enum.CommonYes && !s.executorRegistered(row.Code) {
		return 0, apperror.BadRequest("AI工具编码未注册服务端实现")
	}
	exists, err := repo.ExistsByCode(ctx, row.Code, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具编码失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI工具编码已存在")
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI工具失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id uint64, input MutationInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if _, appErr := getToolOrNotFound(ctx, repo, id); appErr != nil {
		return appErr
	}
	row, appErr := normalizeMutation(input)
	if appErr != nil {
		return appErr
	}
	if row.Status == enum.CommonYes && !s.executorRegistered(row.Code) {
		return apperror.BadRequest("AI工具编码未注册服务端实现")
	}
	exists, err := repo.ExistsByCode(ctx, row.Code, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI工具编码失败", err)
	}
	if exists {
		return apperror.BadRequest("AI工具编码已存在")
	}
	if err := repo.Update(ctx, id, toolUpdateFields(row)); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI工具失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, appErr := getToolOrNotFound(ctx, repo, id)
	if appErr != nil {
		return appErr
	}
	if status == enum.CommonYes && !s.executorRegistered(row.Code) {
		return apperror.BadRequest("AI工具编码未注册服务端实现")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI工具状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI工具ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if _, appErr := getToolOrNotFound(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI工具失败", err)
	}
	return nil
}

func (s *Service) AgentTools(ctx context.Context, agentID uint64) (*AgentToolsResponse, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	exists, err := repo.AgentExists(ctx, agentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if !exists {
		return nil, apperror.NotFound("AI智能体不存在")
	}
	bound, err := repo.ListBoundToolIDs(ctx, agentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询智能体工具绑定失败", err)
	}
	active, err := repo.ListAllActiveToolIDs(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询可绑定AI工具失败", err)
	}
	return &AgentToolsResponse{AgentID: agentID, ToolIDs: uniqueSorted(bound), ActiveToolIDs: uniqueSorted(active)}, nil
}

func (s *Service) UpdateAgentTools(ctx context.Context, agentID uint64, input UpdateAgentToolsInput) *apperror.Error {
	if agentID == 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	exists, err := repo.AgentExists(ctx, agentID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if !exists {
		return apperror.NotFound("AI智能体不存在")
	}
	toolIDs := uniqueSorted(input.ToolIDs)
	activeIDs, err := repo.ListAllActiveToolIDs(ctx)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询可绑定AI工具失败", err)
	}
	activeSet := make(map[uint64]bool, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = true
	}
	for _, id := range toolIDs {
		if !activeSet[id] {
			return apperror.BadRequest("绑定工具不存在或已禁用")
		}
	}
	if err := repo.ReplaceAgentTools(ctx, agentID, toolIDs); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新智能体工具绑定失败", err)
	}
	return nil
}

func (s *Service) ListRuntimeTools(ctx context.Context, agentID uint64) ([]RuntimeTool, *apperror.Error) {
	if agentID == 0 {
		return nil, apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListRuntimeTools(ctx, agentID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询智能体运行工具失败", err)
	}
	out := make([]RuntimeTool, 0, len(rows))
	for _, row := range rows {
		if row.ToolStatus != enum.CommonYes || row.BindingStatus != enum.CommonYes {
			continue
		}
		tool, appErr := runtimeTool(row)
		if appErr != nil {
			return nil, appErr
		}
		out = append(out, tool)
	}
	return out, nil
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (*ExecuteResult, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if input.RunID == 0 || input.Tool.ID == 0 || strings.TrimSpace(input.Tool.Code) == "" {
		return nil, apperror.BadRequest("AI工具调用参数错误")
	}
	startedAt := s.nowTime()
	callID, err := repo.StartToolCall(ctx, StartToolCallInput{RunID: input.RunID, ToolID: input.Tool.ID, ToolCode: input.Tool.Code, ToolName: input.Tool.Name, CallID: input.CallID, ArgumentsJSON: input.Arguments, StartedAt: startedAt})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI工具调用记录失败", err)
	}
	executor := s.executors[strings.TrimSpace(input.Tool.Code)]
	if executor == nil {
		msg := "AI工具服务端实现未注册"
		_ = repo.FinishToolCall(context.Background(), FinishToolCallInput{ID: callID, Status: ToolCallFailed, ErrorMessage: msg, DurationMS: durationMS(startedAt, s.nowTime()), FinishedAt: s.nowTime()})
		return nil, apperror.BadRequest(msg)
	}
	timeout := time.Duration(input.Tool.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, execErr := executor.Execute(toolCtx, input.Arguments)
	finishedAt := s.nowTime()
	if execErr != nil {
		status := ToolCallFailed
		if errors.Is(execErr, context.DeadlineExceeded) || errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
			status = ToolCallTimeout
		}
		_ = repo.FinishToolCall(context.Background(), FinishToolCallInput{ID: callID, Status: status, ErrorMessage: execErr.Error(), DurationMS: durationMS(startedAt, finishedAt), FinishedAt: finishedAt})
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "执行AI工具失败", execErr)
	}
	output, err := json.Marshal(result)
	if err != nil {
		msg := "AI工具结果不是合法JSON"
		_ = repo.FinishToolCall(context.Background(), FinishToolCallInput{ID: callID, Status: ToolCallFailed, ErrorMessage: msg, DurationMS: durationMS(startedAt, finishedAt), FinishedAt: finishedAt})
		return nil, apperror.Wrap(apperror.CodeInternal, 500, msg, err)
	}
	raw := json.RawMessage(output)
	if err := repo.FinishToolCall(context.Background(), FinishToolCallInput{ID: callID, Status: ToolCallSuccess, ResultJSON: &raw, DurationMS: durationMS(startedAt, finishedAt), FinishedAt: finishedAt}); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "更新AI工具调用记录失败", err)
	}
	return &ExecuteResult{CallID: input.CallID, Name: input.Tool.Code, Output: raw}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI工具仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) engineForGenerateAgent(ctx context.Context, agent GenerateAgentConfig) (platformai.Engine, *apperror.Error) {
	if agent.AgentID == 0 || agent.ProviderID == 0 {
		return nil, apperror.BadRequest("AI生成智能体或供应商未配置")
	}
	apiKeyEnc := strings.TrimSpace(agent.EngineAPIKeyEnc)
	if apiKeyEnc == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	apiKey, err := s.secretbox.Decrypt(apiKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	if s.engineFactory == nil {
		return nil, apperror.Internal("AI工具生成引擎工厂未配置")
	}
	engine, err := s.engineFactory.NewEngine(ctx, EngineConfig{EngineType: platformai.EngineType(agent.EngineType), BaseURL: agent.EngineBaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI工具生成引擎失败", err)
	}
	return engine, nil
}

func (s *Service) normalizeGeneratedDraft(draft *GeneratedToolDraft) (*GeneratedToolDraft, *apperror.Error) {
	if draft == nil {
		return nil, apperror.BadRequest("AI生成结果缺少工具草稿")
	}
	row, appErr := normalizeMutation(MutationInput{
		Name:             draft.Name,
		Code:             draft.Code,
		Description:      draft.Description,
		ParametersJSON:   draft.ParametersJSON,
		ResultSchemaJSON: draft.ResultSchemaJSON,
		RiskLevel:        draft.RiskLevel,
		TimeoutMS:        draft.TimeoutMS,
		Status:           draft.Status,
	})
	if appErr != nil {
		return nil, appErr
	}
	if !validToolCode(row.Code) {
		return nil, apperror.BadRequest("AI生成工具编码只能使用小写字母、数字、下划线，长度3到64")
	}
	parameters, appErr := normalizeStrictSchema(json.RawMessage(row.ParametersJSON), "工具参数Schema必须是严格JSON Schema对象")
	if appErr != nil {
		return nil, appErr
	}
	resultSchema, appErr := normalizeStrictSchema(json.RawMessage(row.ResultSchemaJSON), "工具返回Schema必须是严格JSON Schema对象")
	if appErr != nil {
		return nil, appErr
	}
	return &GeneratedToolDraft{
		Name:             row.Name,
		Code:             row.Code,
		Description:      row.Description,
		ParametersJSON:   parameters,
		ResultSchemaJSON: resultSchema,
		RiskLevel:        row.RiskLevel,
		TimeoutMS:        row.TimeoutMS,
		Status:           row.Status,
	}, nil
}

func (s *Service) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func normalizeMutation(input MutationInput) (Tool, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	description := strings.TrimSpace(input.Description)
	riskLevel := strings.TrimSpace(input.RiskLevel)
	if name == "" {
		return Tool{}, apperror.BadRequest("AI工具名称不能为空")
	}
	if code == "" {
		return Tool{}, apperror.BadRequest("AI工具编码不能为空")
	}
	if !isRiskLevel(riskLevel) {
		return Tool{}, apperror.BadRequest("无效的风险等级")
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 30000 {
		return Tool{}, apperror.BadRequest("AI工具超时时间必须在100到30000毫秒之间")
	}
	if !enum.IsCommonStatus(input.Status) {
		return Tool{}, apperror.BadRequest("无效的状态")
	}
	parameters, appErr := normalizeSchema(input.ParametersJSON, "工具参数Schema必须是JSON对象")
	if appErr != nil {
		return Tool{}, appErr
	}
	resultSchema, appErr := normalizeSchema(input.ResultSchemaJSON, "工具返回Schema必须是JSON对象")
	if appErr != nil {
		return Tool{}, appErr
	}
	return Tool{Name: name, Code: code, Description: description, ParametersJSON: parameters, ResultSchemaJSON: resultSchema, RiskLevel: riskLevel, TimeoutMS: input.TimeoutMS, Status: input.Status, IsDel: enum.CommonNo}, nil
}

func normalizeSchema(raw json.RawMessage, msg string) (string, *apperror.Error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", apperror.BadRequest(msg)
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return "", apperror.BadRequest(msg)
	}
	obj, ok := value.(map[string]any)
	if !ok || obj == nil {
		return "", apperror.BadRequest(msg)
	}
	compact, err := json.Marshal(obj)
	if err != nil {
		return "", apperror.BadRequest(msg)
	}
	return string(compact), nil
}

func normalizeStrictSchema(raw json.RawMessage, msg string) (json.RawMessage, *apperror.Error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, apperror.BadRequest(msg)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil || obj == nil {
		return nil, apperror.BadRequest(msg)
	}
	if typ, _ := obj["type"].(string); strings.TrimSpace(typ) != "object" {
		return nil, apperror.BadRequest(msg)
	}
	properties, ok := obj["properties"].(map[string]any)
	if !ok || properties == nil {
		return nil, apperror.BadRequest(msg)
	}
	additional, ok := obj["additionalProperties"].(bool)
	if !ok || additional {
		return nil, apperror.BadRequest(msg)
	}
	required, ok := obj["required"].([]any)
	if !ok {
		if _, exists := obj["required"]; exists {
			return nil, apperror.BadRequest(msg)
		}
		required = []any{}
		obj["required"] = required
	}
	for _, item := range required {
		key, ok := item.(string)
		if !ok || strings.TrimSpace(key) == "" {
			return nil, apperror.BadRequest(msg)
		}
		if _, exists := properties[key]; !exists {
			return nil, apperror.BadRequest(msg)
		}
	}
	compact, err := json.Marshal(obj)
	if err != nil {
		return nil, apperror.BadRequest(msg)
	}
	return json.RawMessage(compact), nil
}

func runtimeTool(row RuntimeToolRow) (RuntimeTool, *apperror.Error) {
	params, appErr := schemaMap(row.ParametersJSON, "AI工具参数Schema损坏")
	if appErr != nil {
		return RuntimeTool{}, appErr
	}
	resultSchema, appErr := schemaMap(row.ResultSchemaJSON, "AI工具返回Schema损坏")
	if appErr != nil {
		return RuntimeTool{}, appErr
	}
	return RuntimeTool{ID: row.ToolID, Name: row.Name, Code: row.Code, Description: row.Description, ParametersJSON: params, ResultSchemaJSON: resultSchema, RiskLevel: row.RiskLevel, TimeoutMS: row.TimeoutMS}, nil
}

func schemaMap(raw string, msg string) (map[string]any, *apperror.Error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &value); err != nil || value == nil {
		return nil, apperror.Internal(msg)
	}
	return value, nil
}

func getToolOrNotFound(ctx context.Context, repo Repository, id uint64) (*Tool, *apperror.Error) {
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI工具失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI工具不存在")
	}
	return row, nil
}

func toolUpdateFields(row Tool) map[string]any {
	return map[string]any{"name": row.Name, "code": row.Code, "description": row.Description, "parameters_json": row.ParametersJSON, "result_schema_json": row.ResultSchemaJSON, "risk_level": row.RiskLevel, "timeout_ms": row.TimeoutMS, "status": row.Status}
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Name = strings.TrimSpace(query.Name)
	query.Code = strings.TrimSpace(query.Code)
	query.RiskLevel = strings.TrimSpace(query.RiskLevel)
	return query
}

func toolDTO(row Tool) ToolDTO {
	return ToolDTO{ID: row.ID, Name: row.Name, Code: row.Code, Description: row.Description, ParametersJSON: rawJSON(row.ParametersJSON), ResultSchemaJSON: rawJSON(row.ResultSchemaJSON), RiskLevel: row.RiskLevel, RiskLevelName: RiskLevelLabels[row.RiskLevel], TimeoutMS: row.TimeoutMS, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func rawJSON(value string) json.RawMessage {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(trimmed)
}

func compactJSON(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return "{}"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func riskOptions() []dict.Option[string] {
	return []dict.Option[string]{{Label: RiskLevelLabels[RiskLow], Value: RiskLow}, {Label: RiskLevelLabels[RiskMedium], Value: RiskMedium}, {Label: RiskLevelLabels[RiskHigh], Value: RiskHigh}}
}

func (s *Service) executorRegistered(value string) bool {
	if s == nil || s.executors == nil {
		return false
	}
	return s.executors[strings.TrimSpace(value)] != nil
}

func decodeGenerateDraftResponse(raw string) (*GenerateDraftResponse, *apperror.Error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(raw))))
	decoder.DisallowUnknownFields()
	var decoded struct {
		OK                  bool                `json:"ok"`
		Draft               *GeneratedToolDraft `json:"draft"`
		Warnings            []string            `json:"warnings"`
		ClarifyingQuestions []string            `json:"clarifying_questions"`
	}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, apperror.BadRequest("AI生成结果不是合法JSON")
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, apperror.BadRequest("AI生成结果不是单个JSON对象")
	}
	response := GenerateDraftResponse{OK: decoded.OK, Draft: decoded.Draft, Warnings: decoded.Warnings, ClarifyingQuestions: decoded.ClarifyingQuestions}
	response.Warnings = nonNilStrings(response.Warnings)
	response.ClarifyingQuestions = nonNilStrings(response.ClarifyingQuestions)
	return &response, nil
}

func buildToolGenerateUserPrompt(requirement string, codeHint string) string {
	builder := strings.Builder{}
	builder.WriteString("管理员要新增一个AI工具，请根据下面需求生成工具草稿。\n\n")
	builder.WriteString("工具需求：\n")
	builder.WriteString(strings.TrimSpace(requirement))
	if strings.TrimSpace(codeHint) != "" {
		builder.WriteString("\n\n工具编码提示：\n")
		builder.WriteString(strings.TrimSpace(codeHint))
	}
	builder.WriteString("\n\n只返回系统提示词要求的JSON。")
	return builder.String()
}

func generateUsage(result *platformai.ChatResult) *GenerateUsage {
	if result == nil || (result.PromptTokens == 0 && result.CompletionTokens == 0 && result.TotalTokens == 0) {
		return nil
	}
	return &GenerateUsage{PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, TotalTokens: result.TotalTokens}
}

func validToolCode(value string) bool {
	code := strings.TrimSpace(value)
	runes := []rune(code)
	if len(runes) < 3 || len(runes) > 64 {
		return false
	}
	for _, r := range runes {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' {
			continue
		}
		return false
	}
	return true
}

func trimStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

type discardEventSink struct{}

func (discardEventSink) Emit(ctx context.Context, event platformai.Event) error { return nil }

func statusText(value int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func uniqueSorted(values []uint64) []uint64 {
	set := make(map[uint64]bool, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = true
		}
	}
	out := make([]uint64, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func isRiskLevel(value string) bool { _, ok := RiskLevelLabels[value]; return ok }

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func durationMS(startedAt time.Time, finishedAt time.Time) uint {
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		return 0
	}
	return uint(finishedAt.Sub(startedAt).Milliseconds())
}

func nowUTC() time.Time { return time.Now() }

func appErrError(appErr *apperror.Error) error {
	if appErr == nil {
		return nil
	}
	return errors.New(appErr.Message)
}
