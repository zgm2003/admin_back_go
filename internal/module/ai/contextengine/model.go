package contextengine

import "time"

type contextPlanRow struct {
	ID                             uint64    `gorm:"column:id;primaryKey"`
	RunID                          uint64    `gorm:"column:run_id"`
	ContextProfileIDSnapshot       *uint64   `gorm:"column:context_profile_id_snapshot"`
	ContextProfileSHA256           []byte    `gorm:"column:context_profile_sha256"`
	ContextIndexGenerationSnapshot *uint64   `gorm:"column:context_index_generation_snapshot"`
	PolicyVersion                  string    `gorm:"column:policy_version"`
	InputFingerprintSHA256         []byte    `gorm:"column:input_fingerprint_sha256"`
	PlanSHA256                     []byte    `gorm:"column:plan_sha256"`
	ModelCapabilitySHA256          []byte    `gorm:"column:model_capability_sha256"`
	APIProtocolSnapshot            string    `gorm:"column:api_protocol_snapshot"`
	TokenCounterIDSnapshot         string    `gorm:"column:token_counter_id_snapshot"`
	ContextWindowTokens            uint64    `gorm:"column:context_window_tokens"`
	EffectiveOutputTokens          uint64    `gorm:"column:effective_output_tokens"`
	ProviderProtocolUpperBound     uint64    `gorm:"column:provider_protocol_upper_bound"`
	ToolContinuationInputReserve   uint64    `gorm:"column:tool_continuation_input_reserve"`
	PolicySafetyMargin             uint64    `gorm:"column:policy_safety_margin"`
	KnownInputBudget               uint64    `gorm:"column:known_input_budget"`
	KnownInputUpperBound           uint64    `gorm:"column:known_input_upper_bound"`
	BudgetProof                    string    `gorm:"column:budget_proof"`
	RetrievalOutcome               string    `gorm:"column:retrieval_outcome"`
	State                          string    `gorm:"column:state"`
	ErrorStage                     *string   `gorm:"column:error_stage"`
	ErrorCode                      *string   `gorm:"column:error_code"`
	ErrorMessage                   *string   `gorm:"column:error_message"`
	MetricsJSON                    string    `gorm:"column:metrics_json"`
	CreatedAt                      time.Time `gorm:"column:created_at"`
}

func (contextPlanRow) TableName() string { return "ai_context_plans" }

type contextPlanItemRow struct {
	ID              uint64    `gorm:"column:id;primaryKey"`
	PlanID          uint64    `gorm:"column:plan_id"`
	Ordinal         uint32    `gorm:"column:ordinal"`
	BlockKind       string    `gorm:"column:block_kind"`
	SourceType      string    `gorm:"column:source_type"`
	SourceRef       string    `gorm:"column:source_ref"`
	SourceSHA256    []byte    `gorm:"column:source_sha256"`
	AtomicGroupKey  string    `gorm:"column:atomic_group_key"`
	Required        uint8     `gorm:"column:required"`
	Priority        int32     `gorm:"column:priority"`
	Decision        string    `gorm:"column:decision"`
	ExclusionReason *string   `gorm:"column:exclusion_reason"`
	TokenUpperBound uint64    `gorm:"column:token_upper_bound"`
	FusionScore     *string   `gorm:"column:fusion_score"`
	RerankScore     *string   `gorm:"column:rerank_score"`
	CitationKey     *string   `gorm:"column:citation_key"`
	ContentSnapshot *string   `gorm:"column:content_snapshot"`
	MetadataJSON    string    `gorm:"column:metadata_json"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (contextPlanItemRow) TableName() string { return "ai_context_plan_items" }

func contextPlanColumnNames() []string {
	return []string{
		"id", "run_id", "context_profile_id_snapshot", "context_profile_sha256",
		"context_index_generation_snapshot", "policy_version", "input_fingerprint_sha256",
		"plan_sha256", "model_capability_sha256", "api_protocol_snapshot",
		"token_counter_id_snapshot", "context_window_tokens", "effective_output_tokens",
		"provider_protocol_upper_bound", "tool_continuation_input_reserve", "policy_safety_margin",
		"known_input_budget", "known_input_upper_bound", "budget_proof", "retrieval_outcome",
		"state", "error_stage", "error_code", "error_message", "metrics_json", "created_at",
	}
}

func contextPlanItemColumnNames() []string {
	return []string{
		"id", "plan_id", "ordinal", "block_kind", "source_type", "source_ref", "source_sha256",
		"atomic_group_key", "required", "priority", "decision", "exclusion_reason",
		"token_upper_bound", "fusion_score", "rerank_score", "citation_key", "content_snapshot",
		"metadata_json", "created_at",
	}
}
