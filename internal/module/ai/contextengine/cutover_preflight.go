package contextengine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
	"gorm.io/gorm"
)

var LegacyKnowledgeTables = [...]string{
	"ai_knowledge_bases",
	"ai_knowledge_documents",
	"ai_knowledge_chunks",
	"ai_agent_knowledge_bases",
	"ai_knowledge_retrievals",
	"ai_knowledge_retrieval_hits",
}

type CutoverViolation struct {
	Code       string `json:"code"`
	ResourceID uint64 `json:"resource_id,omitempty"`
	Detail     string `json:"detail"`
}

type CutoverReport struct {
	ReplyCommandCount uint64             `json:"reply_command_count"`
	ChatAttemptCount  uint64             `json:"chat_attempt_count"`
	LegacyTableCounts map[string]uint64  `json:"legacy_table_counts"`
	CheckedAgentCount uint64             `json:"checked_agent_count"`
	Violations        []CutoverViolation `json:"violations"`
}

type CutoverAgentCapability struct {
	AgentID             uint64
	ProviderID          uint64
	ProviderModelID     uint64
	ModelKind           string
	APIProtocol         string
	ContextWindowTokens int64
	MaxOutputTokens     int64
	TokenCounterID      string
}

type CutoverPreflightRepository interface {
	CountActiveReplyCommands(context.Context) (uint64, error)
	ListActiveChatAttemptIDs(context.Context) ([]uint64, error)
	CountLegacyTable(context.Context, string) (uint64, error)
	ListEnabledChatAgents(context.Context) ([]CutoverAgentCapability, error)
}

type GormCutoverPreflightRepository struct{ db *gorm.DB }

func NewGormCutoverPreflightRepository(db *gorm.DB) *GormCutoverPreflightRepository {
	if db == nil {
		return nil
	}
	return &GormCutoverPreflightRepository{db: db}
}

func (repository *GormCutoverPreflightRepository) CountActiveReplyCommands(ctx context.Context) (uint64, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrPlanRepositoryNotConfigured
	}
	var count int64
	err := repository.db.WithContext(ctx).Table("ai_reply_commands").Where("state IN ?", []string{"claimed", "running", "outcome_unknown"}).Count(&count).Error
	return uint64(count), err
}

func (repository *GormCutoverPreflightRepository) ListActiveChatAttemptIDs(ctx context.Context) ([]uint64, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var ids []uint64
	err := repository.db.WithContext(ctx).Table("ai_provider_attempts").Select("id").Where("state IN ?", []string{"prepared", "dispatched", "outcome_unknown"}).Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}

func (repository *GormCutoverPreflightRepository) CountLegacyTable(ctx context.Context, table string) (uint64, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrPlanRepositoryNotConfigured
	}
	if !isLegacyKnowledgeTable(table) {
		return 0, fmt.Errorf("unsupported legacy table")
	}
	var count int64
	err := repository.db.WithContext(ctx).Table(table).Count(&count).Error
	return uint64(count), err
}

func (repository *GormCutoverPreflightRepository) ListEnabledChatAgents(ctx context.Context) ([]CutoverAgentCapability, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrPlanRepositoryNotConfigured
	}
	var rows []struct {
		AgentID         uint64  `gorm:"column:agent_id"`
		ProviderID      uint64  `gorm:"column:provider_id"`
		ProviderModelID uint64  `gorm:"column:provider_model_id"`
		ModelKind       string  `gorm:"column:model_kind"`
		APIProtocol     string  `gorm:"column:api_protocol"`
		OfficialModelID *string `gorm:"column:official_model_id"`
	}
	query := repository.db.WithContext(ctx).Table("ai_agents AS a").Select(`a.id AS agent_id, p.id AS provider_id, pm.id AS provider_model_id,
		pm.model_kind, p.api_protocol, pm.official_model_id`).Joins("LEFT JOIN ai_providers AS p ON p.id = a.provider_id AND p.status = 1 AND p.is_del = 2").Joins("LEFT JOIN ai_provider_models AS pm ON pm.provider_id = a.provider_id AND pm.model_id = a.model_id AND pm.status = 1").Where("a.status = 1 AND a.is_del = 2").Order("a.id ASC")
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	capabilities := make([]CutoverAgentCapability, len(rows))
	for index, row := range rows {
		capabilities[index] = CutoverAgentCapability{AgentID: row.AgentID, ProviderID: row.ProviderID, ProviderModelID: row.ProviderModelID, ModelKind: row.ModelKind, APIProtocol: row.APIProtocol}
		if row.OfficialModelID != nil {
			if model, resolveErr := officialmodel.Default.ResolveIdentity(strings.TrimSpace(*row.OfficialModelID)); resolveErr == nil {
				capabilities[index].ContextWindowTokens = model.ContextWindowTokens
				capabilities[index].MaxOutputTokens = model.MaxOutputTokens
				capabilities[index].TokenCounterID = model.TokenCounterID
			}
		}
	}
	return capabilities, nil
}

func RunCutoverPreflight(ctx context.Context, repository CutoverPreflightRepository) (CutoverReport, error) {
	if repository == nil {
		return CutoverReport{}, ErrPlanRepositoryNotConfigured
	}
	report := CutoverReport{LegacyTableCounts: make(map[string]uint64, len(LegacyKnowledgeTables))}
	var err error
	if report.ReplyCommandCount, err = repository.CountActiveReplyCommands(ctx); err != nil {
		return CutoverReport{}, fmt.Errorf("count active reply commands: %w", err)
	}
	if report.ReplyCommandCount != 0 {
		report.Violations = append(report.Violations, CutoverViolation{Code: "active_reply_commands", Detail: "active reply command count is non-zero"})
	}
	attemptIDs, err := repository.ListActiveChatAttemptIDs(ctx)
	if err != nil {
		return CutoverReport{}, fmt.Errorf("list active chat attempts: %w", err)
	}
	report.ChatAttemptCount = uint64(len(attemptIDs))
	for _, id := range attemptIDs {
		report.Violations = append(report.Violations, CutoverViolation{Code: "active_chat_attempts", ResourceID: id, Detail: "chat attempt is not terminal"})
	}
	for _, table := range LegacyKnowledgeTables {
		count, countErr := repository.CountLegacyTable(ctx, table)
		if countErr != nil {
			return CutoverReport{}, fmt.Errorf("count legacy table: %w", countErr)
		}
		report.LegacyTableCounts[table] = count
		if count != 0 {
			report.Violations = append(report.Violations, CutoverViolation{Code: "legacy_table_not_empty", Detail: table + " contains rows"})
		}
	}
	agents, err := repository.ListEnabledChatAgents(ctx)
	if err != nil {
		return CutoverReport{}, fmt.Errorf("list enabled chat agents: %w", err)
	}
	report.CheckedAgentCount = uint64(len(agents))
	for _, agent := range agents {
		validateCutoverAgent(&report, agent)
	}
	sortCutoverViolations(&report)
	return report, nil
}

func validateCutoverAgent(report *CutoverReport, agent CutoverAgentCapability) {
	if agent.ProviderID == 0 || agent.ProviderModelID == 0 {
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_provider_model_missing", ResourceID: agent.AgentID, Detail: "provider or model is missing"})
	}
	if strings.TrimSpace(agent.ModelKind) != "chat" {
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_model_kind_invalid", ResourceID: agent.AgentID, Detail: "enabled agent model is not chat"})
	}
	if agent.APIProtocol != APIProtocolChatCompletions && agent.APIProtocol != APIProtocolResponses {
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_api_protocol_invalid", ResourceID: agent.AgentID, Detail: "provider API protocol is unsupported"})
	}
	if agent.ContextWindowTokens <= 0 || agent.MaxOutputTokens <= 0 || agent.MaxOutputTokens > agent.ContextWindowTokens {
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_model_limits_invalid", ResourceID: agent.AgentID, Detail: "model context or output limits are invalid"})
	}
	if strings.TrimSpace(agent.TokenCounterID) == "" {
		// Empty is a missing catalog capability. Unknown IDs are also rejected by
		// the shared registry below; neither case receives a guessed default.
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_token_counter_invalid", ResourceID: agent.AgentID, Detail: "token counter is not registered"})
	} else if _, err := infraai.ResolveTokenCounter(agent.TokenCounterID); err != nil {
		report.Violations = append(report.Violations, CutoverViolation{Code: "agent_token_counter_invalid", ResourceID: agent.AgentID, Detail: "token counter is not registered"})
	}
}

func isLegacyKnowledgeTable(value string) bool {
	for _, table := range LegacyKnowledgeTables {
		if value == table {
			return true
		}
	}
	return false
}

func sortCutoverViolations(report *CutoverReport) {
	sort.SliceStable(report.Violations, func(i, j int) bool {
		if report.Violations[i].Code != report.Violations[j].Code {
			return report.Violations[i].Code < report.Violations[j].Code
		}
		return report.Violations[i].ResourceID < report.Violations[j].ResourceID
	})
}
