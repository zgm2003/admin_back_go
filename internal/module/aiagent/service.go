package aiagent

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	models, err := repo.InitModels(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI模型选项失败", err)
	}
	knowledge, err := repo.InitKnowledgeBases(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询知识库选项失败", err)
	}
	modelOptions := make([]dict.Option[int], 0, len(models))
	for _, row := range models {
		modelOptions = append(modelOptions, dict.Option[int]{Value: int(row.ID), Label: row.Name + " (" + enum.AIDriverLabels[row.Driver] + ")"})
	}
	knowledgeOptions := make([]dict.Option[int], 0, len(knowledge))
	for _, row := range knowledge {
		knowledgeOptions = append(knowledgeOptions, dict.Option[int]{Value: int(row.ID), Label: row.Name})
	}
	return &InitResponse{Dict: InitDict{AIModeArr: dict.AIModeOptions(), AICapabilityArr: dict.AICapabilityOptions(), AISceneArr: []dict.Option[string]{}, CommonStatusArr: dict.CommonStatusOptions(), ModelList: modelOptions, KnowledgeBaseList: knowledgeOptions}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Agent.ID)
	}
	bindings, err := repo.BindingData(ctx, ids)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体绑定失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row, bindings))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Create(ctx context.Context, input MutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	normalized, appErr := s.normalizeMutation(ctx, repo, input, true)
	if appErr != nil {
		return 0, appErr
	}
	var id int64
	err := repo.WithTx(ctx, func(tx Repository) error {
		var err error
		id, err = tx.CreateAgent(ctx, normalized.agent)
		if err != nil {
			return err
		}
		if err := tx.SyncSceneBindings(ctx, id, normalized.sceneCodes); err != nil {
			return err
		}
		if err := tx.SyncToolBindings(ctx, id, normalized.toolIDs); err != nil {
			return err
		}
		return tx.SyncKnowledgeBindings(ctx, id, normalized.knowledgeIDs)
	})
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI智能体失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input MutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	normalized, appErr := s.normalizeMutation(ctx, repo, input, false)
	if appErr != nil {
		return appErr
	}
	fields := agentFields(normalized.agent)
	err = repo.WithTx(ctx, func(tx Repository) error {
		if err := tx.UpdateAgent(ctx, id, fields); err != nil {
			return err
		}
		if err := tx.SyncSceneBindings(ctx, id, normalized.sceneCodes); err != nil {
			return err
		}
		if err := tx.SyncToolBindings(ctx, id, normalized.toolIDs); err != nil {
			return err
		}
		return tx.SyncKnowledgeBindings(ctx, id, normalized.knowledgeIDs)
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI智能体失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI智能体状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的AI智能体ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI智能体失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI智能体不存在")
	}
	if err := repo.SoftDeleteAgentAndBindings(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI智能体失败", err)
	}
	return nil
}

type normalizedMutation struct {
	agent        Agent
	sceneCodes   []string
	toolIDs      []int64
	knowledgeIDs []int64
}

func (s *Service) normalizeMutation(ctx context.Context, repo Repository, input MutationInput, creating bool) (normalizedMutation, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return normalizedMutation{}, apperror.BadRequest("智能体名称不能为空")
	}
	if len([]rune(name)) > 50 {
		return normalizedMutation{}, apperror.BadRequest("智能体名称不能超过50个字符")
	}
	if input.ModelID <= 0 {
		return normalizedMutation{}, apperror.BadRequest("模型ID不能为空")
	}
	ok, err := repo.ActiveModelExists(ctx, input.ModelID)
	if err != nil {
		return normalizedMutation{}, apperror.Wrap(apperror.CodeInternal, 500, "校验AI模型失败", err)
	}
	if !ok {
		return normalizedMutation{}, apperror.BadRequest("关联的模型不存在或已禁用")
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = enum.AIModeChat
	}
	if !enum.IsAIMode(mode) {
		return normalizedMutation{}, apperror.BadRequest("无效的智能体模式")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedMutation{}, apperror.BadRequest("无效的状态")
	}
	sceneCodes, appErr := normalizeSceneCodes(input.SceneCodes, input.Scene)
	if appErr != nil {
		return normalizedMutation{}, appErr
	}
	toolIDs := normalizeIDs(input.ToolIDs)
	knowledgeIDs := normalizeIDs(input.KnowledgeBaseIDs)
	if appErr := validateActiveIDs(ctx, repo.ActiveToolIDs, toolIDs, "工具不存在或已禁用"); appErr != nil {
		return normalizedMutation{}, appErr
	}
	if appErr := validateActiveIDs(ctx, repo.ActiveKnowledgeBaseIDs, knowledgeIDs, "知识库不存在或已禁用"); appErr != nil {
		return normalizedMutation{}, appErr
	}
	capabilities := normalizeCapabilities(input.Capabilities, mode, len(toolIDs) > 0, len(knowledgeIDs) > 0)
	capJSON := mustJSON(capabilities)
	runtimeJSON := nullableJSON(input.RuntimeConfig)
	policyJSON := nullableJSON(input.Policy)
	legacyScene := firstScene(sceneCodes)
	return normalizedMutation{agent: Agent{Name: name, ModelID: input.ModelID, Avatar: trimPtr(input.Avatar), SystemPrompt: input.SystemPrompt, Mode: mode, Scene: legacyScene, CapabilitiesJSON: &capJSON, RuntimeConfigJSON: runtimeJSON, PolicyJSON: policyJSON, Status: status, IsDel: enum.CommonNo}, sceneCodes: sceneCodes, toolIDs: toolIDs, knowledgeIDs: knowledgeIDs}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI智能体仓储未配置")
	}
	return s.repository, nil
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
	query.Mode = strings.TrimSpace(query.Mode)
	return query
}
func totalPage(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
func normalizeIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	out := []int64{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
func normalizeSceneCodes(values []string, legacy *string) ([]string, *apperror.Error) {
	raw := append([]string{}, values...)
	if legacy != nil && strings.TrimSpace(*legacy) != "" {
		raw = append([]string{strings.TrimSpace(*legacy)}, raw...)
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if enum.IsRetiredAIScene(item) {
			return nil, apperror.BadRequest("已下线的AI场景不能再绑定智能体")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}
func firstScene(values []string) *string {
	if len(values) == 0 {
		return nil
	}
	value := values[0]
	return &value
}
func validateActiveIDs(ctx context.Context, fn func(context.Context, []int64) (map[int64]struct{}, error), ids []int64, msg string) *apperror.Error {
	if len(ids) == 0 {
		return nil
	}
	active, err := fn(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, msg, err)
	}
	for _, id := range ids {
		if _, ok := active[id]; !ok {
			return apperror.BadRequest(msg)
		}
	}
	return nil
}
func normalizeCapabilities(input Capabilities, mode string, hasTools bool, hasKnowledge bool) Capabilities {
	input.Chat = true
	if mode == enum.AIModeTool || hasTools {
		input.Tools = true
	}
	if mode == enum.AIModeRAG || hasKnowledge {
		input.RAG = true
	}
	if mode == enum.AIModeWorkflow {
		input.Workflow = true
	}
	return input
}
func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
func nullableJSON(value JSONObject) *string {
	if len(value) == 0 {
		return nil
	}
	s := mustJSON(value)
	return &s
}
func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
func agentFields(row Agent) map[string]any {
	return map[string]any{"name": row.Name, "model_id": row.ModelID, "avatar": row.Avatar, "system_prompt": row.SystemPrompt, "mode": row.Mode, "scene": row.Scene, "capabilities_json": row.CapabilitiesJSON, "runtime_config_json": row.RuntimeConfigJSON, "policy_json": row.PolicyJSON, "status": row.Status}
}
func listItem(row ListRow, bindings BindingData) ListItem {
	a := row.Agent
	sceneCodes := bindings.SceneCodes[a.ID]
	if len(sceneCodes) == 0 && a.Scene != nil && *a.Scene != "" {
		sceneCodes = []string{*a.Scene}
	}
	sceneNames := make([]string, 0, len(sceneCodes))
	for _, code := range sceneCodes {
		sceneNames = append(sceneNames, enum.RetiredAISceneLabels[code])
	}
	caps := decodeCapabilities(a.CapabilitiesJSON, a.Mode)
	return ListItem{ID: a.ID, Name: a.Name, ModelID: a.ModelID, ModelName: row.ModelName, ModelDeleted: row.ModelDeleted, Driver: row.Driver, DriverName: enum.AIDriverLabels[row.Driver], ModelCode: row.ModelCode, Avatar: a.Avatar, SystemPrompt: a.SystemPrompt, Mode: a.Mode, ModeName: enum.AIModeLabels[a.Mode], Scene: firstScene(sceneCodes), SceneName: firstString(sceneNames), SceneCodes: sceneCodes, SceneNames: sceneNames, Capabilities: caps, RuntimeConfig: decodeObject(a.RuntimeConfigJSON), Policy: decodeObject(a.PolicyJSON), KnowledgeBaseIDs: bindings.KnowledgeBaseIDs[a.ID], KnowledgeBaseNames: bindings.KnowledgeBaseNames[a.ID], Status: a.Status, StatusName: statusName(a.Status), CreatedAt: a.CreatedAt.Format(timeLayout), UpdatedAt: a.UpdatedAt.Format(timeLayout)}
}
func decodeCapabilities(raw *string, mode string) Capabilities {
	var c Capabilities
	if raw != nil {
		_ = json.Unmarshal([]byte(*raw), &c)
	}
	return normalizeCapabilities(c, mode, false, false)
}
func decodeObject(raw *string) JSONObject {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return JSONObject{}
	}
	var out JSONObject
	if err := json.Unmarshal([]byte(*raw), &out); err != nil {
		return JSONObject{}
	}
	return out
}
func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
func statusName(value int) string {
	if value == enum.CommonYes {
		return "启用"
	}
	if value == enum.CommonNo {
		return "禁用"
	}
	return ""
}
