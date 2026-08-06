package contextengine

import (
	"context"
	"encoding/json"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

const (
	ContextPolicyV1                  = "context_policy_v1"
	contextProtocolBaseReserve int64 = 2048
	contextToolReserve         int64 = 4096
	contextSafetyMargin        int64 = 4096
)

type GormRuntimeFactsReader struct {
	db       *gorm.DB
	resolver officialmodel.Resolver
}

func NewRuntimeFactsReader(client *database.Client, resolver officialmodel.Resolver) *GormRuntimeFactsReader {
	if client == nil || client.Gorm == nil || resolver == nil {
		return nil
	}
	return &GormRuntimeFactsReader{db: client.Gorm, resolver: resolver}
}

type runtimeIdentityRow struct {
	AgentID               uint64                      `gorm:"column:agent_id"`
	AgentProviderID       uint64                      `gorm:"column:agent_provider_id"`
	AgentModelID          string                      `gorm:"column:agent_model_id"`
	AgentSystemPrompt     string                      `gorm:"column:agent_system_prompt"`
	AgentContextProfileID *uint64                     `gorm:"column:agent_context_profile_id"`
	AgentStatus           int                         `gorm:"column:agent_status"`
	AgentDeleted          int                         `gorm:"column:agent_deleted"`
	ProviderID            uint64                      `gorm:"column:provider_id"`
	ProviderEngineType    string                      `gorm:"column:provider_engine_type"`
	ProviderBaseURL       string                      `gorm:"column:provider_base_url"`
	ProviderAPIProtocol   string                      `gorm:"column:provider_api_protocol"`
	ProviderStatus        int                         `gorm:"column:provider_status"`
	ProviderDeleted       int                         `gorm:"column:provider_deleted"`
	ProviderModelID       uint64                      `gorm:"column:provider_model_id"`
	ProviderModelKind     string                      `gorm:"column:provider_model_kind"`
	ProviderModelStatus   int                         `gorm:"column:provider_model_status"`
	OfficialModelID       *string                     `gorm:"column:official_model_id"`
	OfficialCatalog       *string                     `gorm:"column:official_catalog_version"`
	MappingStatus         officialmodel.MappingStatus `gorm:"column:mapping_status"`
}

type runtimeMessageRow struct {
	ID             uint64  `gorm:"column:id"`
	ConversationID uint64  `gorm:"column:conversation_id"`
	Role           int     `gorm:"column:role"`
	Content        string  `gorm:"column:content"`
	MetaJSON       *string `gorm:"column:meta_json"`
	UserID         uint64  `gorm:"column:user_id"`
	AgentID        uint64  `gorm:"column:agent_id"`
}

type runtimeBindingRow struct {
	ID        uint64 `gorm:"column:id"`
	SpaceID   uint64 `gorm:"column:space_id"`
	ProfileID uint64 `gorm:"column:profile_id"`
}

type runtimeToolRow struct {
	BindingID      uint64 `gorm:"column:binding_id"`
	ToolID         uint64 `gorm:"column:tool_id"`
	Code           string `gorm:"column:code"`
	Description    string `gorm:"column:description"`
	ParametersJSON string `gorm:"column:parameters_json"`
}

func (reader *GormRuntimeFactsReader) LoadRuntimeFacts(ctx context.Context, input RuntimeInput) (RuntimeFacts, error) {
	if reader == nil || reader.db == nil || reader.resolver == nil {
		return RuntimeFacts{}, ErrPlanRepositoryNotConfigured
	}
	identity, err := reader.loadIdentity(ctx, input)
	if err != nil {
		return RuntimeFacts{}, err
	}
	resolved, err := officialmodel.ResolveMappedRoute(ctx, reader.resolver, identity.AgentModelID, optionalString(identity.OfficialModelID), optionalString(identity.OfficialCatalog), identity.MappingStatus)
	if err != nil {
		return RuntimeFacts{}, err
	}
	message, err := reader.loadCurrentMessage(ctx, input)
	if err != nil {
		return RuntimeFacts{}, err
	}
	attachments, err := runtimeAttachments(message.MetaJSON)
	if err != nil {
		return RuntimeFacts{}, err
	}
	bindings, err := reader.loadBindings(ctx, input.AgentID, identity.AgentContextProfileID)
	if err != nil {
		return RuntimeFacts{}, err
	}
	tools, definitions, err := reader.loadTools(ctx, input.AgentID)
	if err != nil {
		return RuntimeFacts{}, err
	}
	if !sameToolDefinitions(input.Tools, definitions) {
		return RuntimeFacts{}, ErrInvalidContextPlan
	}
	modelCapability, err := runtimeModelCapability(identity, resolved)
	if err != nil {
		return RuntimeFacts{}, err
	}
	fingerprint, profileSnapshot, profile, retrieval, err := reader.runtimeFingerprint(ctx, input, identity, message, attachments, bindings, tools, modelCapability, resolved)
	if err != nil {
		return RuntimeFacts{}, err
	}
	budget, err := runtimeBudget(resolved.Model, len(tools) > 0, attachments)
	if err != nil {
		return RuntimeFacts{}, err
	}
	groups, err := runtimeCoreGroups(input.CurrentMessageID, identity.AgentID, identity.AgentSystemPrompt, fingerprint.AgentSHA256, message.Content, attachments, tools, modelCapability.TokenCounterID)
	if err != nil {
		return RuntimeFacts{}, err
	}
	if profile != nil {
		retrieval = &RuntimeRetrievalFacts{Profile: *profile, SpaceIDs: bindingSpaceIDs(bindings), CurrentText: message.Content, HasSources: retrieval != nil && retrieval.HasSources}
	}
	return RuntimeFacts{Fingerprint: fingerprint, ModelCapability: modelCapability, Budget: budget, Profile: profileSnapshot, CoreGroups: groups, Retrieval: retrieval}, nil
}

func (reader *GormRuntimeFactsReader) loadIdentity(ctx context.Context, input RuntimeInput) (runtimeIdentityRow, error) {
	row, err := reader.loadActiveIdentity(ctx, input.AgentID)
	if err != nil {
		return runtimeIdentityRow{}, err
	}
	if row.AgentProviderID != input.ProviderID || row.ProviderID != input.ProviderID ||
		row.AgentModelID != input.ModelID || row.ProviderAPIProtocol != input.APIProtocol {
		return runtimeIdentityRow{}, ErrInvalidContextPlan
	}
	return row, nil
}

func (reader *GormRuntimeFactsReader) loadActiveIdentity(ctx context.Context, agentID uint64) (runtimeIdentityRow, error) {
	if reader == nil || reader.db == nil || agentID == 0 {
		return runtimeIdentityRow{}, ErrPlanRepositoryNotConfigured
	}
	var row runtimeIdentityRow
	err := reader.db.WithContext(ctx).Table("ai_agents AS agent").
		Select(`agent.id AS agent_id, agent.provider_id AS agent_provider_id, agent.model_id AS agent_model_id,
			agent.system_prompt AS agent_system_prompt, agent.context_profile_id AS agent_context_profile_id,
			agent.status AS agent_status, agent.is_del AS agent_deleted,
			provider.id AS provider_id, provider.engine_type AS provider_engine_type, provider.base_url AS provider_base_url,
			provider.api_protocol AS provider_api_protocol, provider.status AS provider_status, provider.is_del AS provider_deleted,
			model.id AS provider_model_id, model.model_kind AS provider_model_kind, model.status AS provider_model_status,
			model.official_model_id, model.official_catalog_version, model.mapping_status`).
		Joins("JOIN ai_providers AS provider ON provider.id = agent.provider_id").
		Joins("JOIN ai_provider_models AS model ON model.provider_id = agent.provider_id AND model.model_id = agent.model_id").
		Where("agent.id = ?", agentID).Take(&row).Error
	if err != nil {
		return runtimeIdentityRow{}, err
	}
	if row.AgentID != agentID || row.AgentStatus != enum.CommonYes || row.AgentDeleted != enum.CommonNo || row.ProviderStatus != enum.CommonYes ||
		row.ProviderDeleted != enum.CommonNo || row.ProviderModelStatus != enum.CommonYes || row.ProviderModelKind != string(aiprovider.ModelKindChat) {
		return runtimeIdentityRow{}, ErrInvalidContextPlan
	}
	return row, nil
}

func (reader *GormRuntimeFactsReader) loadCurrentMessage(ctx context.Context, input RuntimeInput) (runtimeMessageRow, error) {
	var row runtimeMessageRow
	err := reader.db.WithContext(ctx).Table("ai_messages AS message").
		Select("message.id, message.conversation_id, message.role, message.content, message.meta_json, conversation.user_id, conversation.agent_id").
		Joins("JOIN ai_conversations AS conversation ON conversation.id = message.conversation_id AND conversation.is_del = ?", enum.CommonNo).
		Where("message.id = ? AND message.is_del = ?", input.CurrentMessageID, enum.CommonNo).Take(&row).Error
	if err != nil {
		return runtimeMessageRow{}, err
	}
	if row.ID != input.CurrentMessageID || row.ConversationID != input.ConversationID || row.UserID != input.UserID ||
		row.AgentID != input.AgentID || row.Role != enum.AIMessageRoleUser {
		return runtimeMessageRow{}, ErrInvalidContextPlan
	}
	return row, nil
}

func (reader *GormRuntimeFactsReader) loadBindings(ctx context.Context, agentID uint64, profileID *uint64) ([]runtimeBindingRow, error) {
	var rows []runtimeBindingRow
	query := reader.db.WithContext(ctx).Table("ai_context_bindings AS binding").
		Select("binding.id, binding.space_id, space.profile_id").
		Joins("JOIN ai_context_spaces AS space ON space.id = binding.space_id AND space.deleted_at IS NULL").
		Where("binding.agent_id = ? AND binding.status = ? AND space.status = ?", agentID, "enabled", SpaceEnabled)
	if profileID == nil {
		query = query.Where("1 = 0")
	} else {
		query = query.Where("space.profile_id = ?", *profileID)
	}
	if err := query.Order("binding.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (reader *GormRuntimeFactsReader) loadTools(ctx context.Context, agentID uint64) ([]runtimeToolRow, []infraai.ToolDefinition, error) {
	var rows []runtimeToolRow
	err := reader.db.WithContext(ctx).Table("ai_agent_tools AS binding").
		Select("binding.id AS binding_id, tool.id AS tool_id, tool.code, tool.description, tool.parameters_json").
		Joins("JOIN ai_tools AS tool ON tool.id = binding.tool_id").
		Where("binding.agent_id = ? AND binding.status = ? AND tool.status = ? AND tool.is_del = ?", agentID, enum.CommonYes, enum.CommonYes, enum.CommonNo).
		Order("binding.id ASC").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	definitions := make([]infraai.ToolDefinition, len(rows))
	for index, row := range rows {
		var parameters map[string]any
		if err := json.Unmarshal([]byte(row.ParametersJSON), &parameters); err != nil || parameters == nil {
			return nil, nil, ErrInvalidContextPlan
		}
		definitions[index] = infraai.ToolDefinition{Name: row.Code, Description: row.Description, Parameters: parameters}
	}
	return rows, definitions, nil
}

var _ RuntimeFactsReader = (*GormRuntimeFactsReader)(nil)
