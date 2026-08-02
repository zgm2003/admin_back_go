package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const (
	ContextPlanMetricsSchemaV1   = "context_plan_metrics_v1"
	ContextBlockMetadataSchemaV1 = "context_block_metadata_v1"
	ContextLocatorSchemaV1       = "context_locator_v1"
	RetrievalBranchesSchemaV1    = "retrieval_branches_v1"

	APIProtocolChatCompletions = "chat_completions"
	APIProtocolResponses       = "responses"
)

type RetrievalOutcome string

const (
	RetrievalSkipped RetrievalOutcome = "skipped"
	RetrievalNoHit   RetrievalOutcome = "no_hit"
	RetrievalHit     RetrievalOutcome = "hit"
	RetrievalFailed  RetrievalOutcome = "failed"
)

func (outcome RetrievalOutcome) Validate() error {
	switch outcome {
	case RetrievalSkipped, RetrievalNoHit, RetrievalHit, RetrievalFailed:
		return nil
	}
	return invalidValue("retrieval outcome", string(outcome))
}

type PlanState string

const (
	PlanReady  PlanState = "ready"
	PlanFailed PlanState = "failed"
)

func (state PlanState) Validate() error {
	switch state {
	case PlanReady, PlanFailed:
		return nil
	}
	return invalidValue("plan state", string(state))
}

type BudgetProof string

const (
	BudgetExact            BudgetProof = "exact"
	BudgetConservative     BudgetProof = "conservative"
	BudgetOpaqueAttachment BudgetProof = "opaque_attachment"
)

func (proof BudgetProof) Validate() error {
	switch proof {
	case BudgetExact, BudgetConservative, BudgetOpaqueAttachment:
		return nil
	}
	return invalidValue("budget proof", string(proof))
}

type BlockKind string

const (
	BlockSystemInstruction  BlockKind = "system_instruction"
	BlockCurrentUserMessage BlockKind = "current_user_message"
	BlockCurrentAttachment  BlockKind = "current_attachment"
	BlockRecentTurn         BlockKind = "recent_turn"
	BlockRecalledTurn       BlockKind = "recalled_turn"
	BlockHistoryAttachment  BlockKind = "history_attachment"
	BlockConversationMemory BlockKind = "conversation_memory"
	BlockDocumentEvidence   BlockKind = "document_evidence"
	BlockToolDefinition     BlockKind = "tool_definition"
	BlockToolCall           BlockKind = "tool_call"
	BlockToolResult         BlockKind = "tool_result"
)

func (kind BlockKind) Validate() error {
	switch kind {
	case BlockSystemInstruction,
		BlockCurrentUserMessage,
		BlockCurrentAttachment,
		BlockRecentTurn,
		BlockRecalledTurn,
		BlockHistoryAttachment,
		BlockConversationMemory,
		BlockDocumentEvidence,
		BlockToolDefinition,
		BlockToolCall,
		BlockToolResult:
		return nil
	}
	return invalidValue("block kind", string(kind))
}

func (kind BlockKind) isAttachment() bool {
	return kind == BlockCurrentAttachment || kind == BlockHistoryAttachment
}

type Decision string

const (
	DecisionSelected Decision = "selected"
	DecisionExcluded Decision = "excluded"
)

func (decision Decision) Validate() error {
	switch decision {
	case DecisionSelected, DecisionExcluded:
		return nil
	}
	return invalidValue("decision", string(decision))
}

type ExclusionReason string

const (
	ExclusionBudgetExceeded          ExclusionReason = "budget_exceeded"
	ExclusionDuplicateContent        ExclusionReason = "duplicate_content"
	ExclusionBelowRelevanceThreshold ExclusionReason = "below_relevance_threshold"
	ExclusionSupersededMemory        ExclusionReason = "superseded_memory"
	ExclusionInactiveSource          ExclusionReason = "inactive_source"
	ExclusionPermissionChanged       ExclusionReason = "permission_changed"
	ExclusionUnsupportedAttachment   ExclusionReason = "unsupported_attachment"
)

func (reason ExclusionReason) Validate() error {
	switch reason {
	case ExclusionBudgetExceeded,
		ExclusionDuplicateContent,
		ExclusionBelowRelevanceThreshold,
		ExclusionSupersededMemory,
		ExclusionInactiveSource,
		ExclusionPermissionChanged,
		ExclusionUnsupportedAttachment:
		return nil
	}
	return invalidValue("exclusion reason", string(reason))
}

type ProfileIndexState string

const (
	ProfileIndexProvisioning ProfileIndexState = "provisioning"
	ProfileIndexReady        ProfileIndexState = "ready"
	ProfileIndexRebuilding   ProfileIndexState = "rebuilding"
	ProfileIndexFailed       ProfileIndexState = "failed"
)

func (state ProfileIndexState) Validate() error {
	switch state {
	case ProfileIndexProvisioning, ProfileIndexReady, ProfileIndexRebuilding, ProfileIndexFailed:
		return nil
	}
	return invalidValue("profile index state", string(state))
}

type ProfileIndex struct {
	State            ProfileIndexState
	ActiveGeneration *uint64
	TargetGeneration *uint64
	ErrorCode        *ErrorCode
}

func (index ProfileIndex) Validate() error {
	if err := index.State.Validate(); err != nil {
		return err
	}
	if invalidGeneration(index.ActiveGeneration) || invalidGeneration(index.TargetGeneration) {
		return ErrInvalidProfileIndex
	}
	if index.ActiveGeneration != nil && index.TargetGeneration != nil && *index.TargetGeneration <= *index.ActiveGeneration {
		return ErrInvalidProfileIndex
	}
	switch index.State {
	case ProfileIndexProvisioning:
		if index.ActiveGeneration != nil || index.TargetGeneration == nil || index.ErrorCode != nil {
			return ErrInvalidProfileIndex
		}
	case ProfileIndexReady:
		if index.ActiveGeneration == nil || index.TargetGeneration != nil || invalidProfileIndexError(index.ErrorCode) {
			return ErrInvalidProfileIndex
		}
	case ProfileIndexRebuilding:
		if index.TargetGeneration == nil || index.ErrorCode != nil {
			return ErrInvalidProfileIndex
		}
	case ProfileIndexFailed:
		if index.ErrorCode == nil || invalidProfileIndexError(index.ErrorCode) {
			return ErrInvalidProfileIndex
		}
	}
	return nil
}

func (index ProfileIndex) ValidateTransition(next ProfileIndex) error {
	if err := index.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	switch index.State {
	case ProfileIndexProvisioning:
		if next.State == ProfileIndexReady && next.ErrorCode == nil && equalGeneration(next.ActiveGeneration, index.TargetGeneration) {
			return nil
		}
		if next.State == ProfileIndexFailed && next.ActiveGeneration == nil && equalGeneration(next.TargetGeneration, index.TargetGeneration) {
			return nil
		}
	case ProfileIndexReady:
		if next.State == ProfileIndexRebuilding && equalGeneration(next.ActiveGeneration, index.ActiveGeneration) {
			return nil
		}
		if next.State == ProfileIndexFailed && next.TargetGeneration == nil && next.ErrorCode != nil &&
			*next.ErrorCode == ErrCodeIndexInconsistent && equalGeneration(next.ActiveGeneration, index.ActiveGeneration) {
			return nil
		}
	case ProfileIndexRebuilding:
		if next.State == ProfileIndexReady && next.ErrorCode == nil && equalGeneration(next.ActiveGeneration, index.TargetGeneration) {
			return nil
		}
		if next.State == ProfileIndexReady && next.ErrorCode != nil && equalGeneration(next.ActiveGeneration, index.ActiveGeneration) {
			return nil
		}
		if index.ActiveGeneration == nil && next.State == ProfileIndexFailed && next.ActiveGeneration == nil &&
			equalGeneration(next.TargetGeneration, index.TargetGeneration) {
			return nil
		}
	case ProfileIndexFailed:
		if next.State == ProfileIndexRebuilding && equalGeneration(next.ActiveGeneration, index.ActiveGeneration) &&
			generationAfter(next.TargetGeneration, index.ActiveGeneration, index.TargetGeneration) {
			return nil
		}
	}
	return ErrInvalidProfileIndex
}

type Budget struct {
	ContextWindowTokens          int64       `json:"context_window_tokens"`
	EffectiveOutputTokens        int64       `json:"effective_output_tokens"`
	ProviderProtocolUpperBound   int64       `json:"provider_protocol_upper_bound"`
	ToolContinuationInputReserve int64       `json:"tool_continuation_input_reserve"`
	PolicySafetyMargin           int64       `json:"policy_safety_margin"`
	KnownInputBudget             int64       `json:"known_input_budget"`
	KnownInputUpperBound         int64       `json:"known_input_upper_bound"`
	Proof                        BudgetProof `json:"proof"`
}

func (budget Budget) Validate() error {
	if err := budget.Proof.Validate(); err != nil {
		return err
	}
	if budget.ContextWindowTokens <= 0 || budget.EffectiveOutputTokens <= 0 ||
		budget.ProviderProtocolUpperBound < 0 || budget.ToolContinuationInputReserve < 0 ||
		budget.PolicySafetyMargin < 0 || budget.KnownInputBudget < 0 || budget.KnownInputUpperBound < 0 {
		return ErrInvalidBudget
	}
	want := budget.ContextWindowTokens - budget.EffectiveOutputTokens - budget.ProviderProtocolUpperBound - budget.PolicySafetyMargin
	if want < 0 || budget.KnownInputBudget != want ||
		budget.ToolContinuationInputReserve > budget.ProviderProtocolUpperBound ||
		budget.KnownInputUpperBound > budget.KnownInputBudget {
		return ErrInvalidBudget
	}
	return nil
}

type ProfileSnapshot struct {
	ID              uint64
	SHA256          [sha256.Size]byte
	IndexGeneration *uint64
}

func (snapshot ProfileSnapshot) Validate() error {
	if snapshot.ID == 0 || isZeroSHA256(snapshot.SHA256) || invalidGeneration(snapshot.IndexGeneration) {
		return fmt.Errorf("%w: profile snapshot", ErrInvalidContextPlan)
	}
	return nil
}

type PlanError struct {
	Stage   string
	Code    ErrorCode
	Message *string
}

func (planError PlanError) Validate() error {
	if !validIdentifier(planError.Stage, 64) || planError.Code.Validate() != nil || invalidOptionalString(planError.Message) {
		return fmt.Errorf("%w: plan error", ErrInvalidContextPlan)
	}
	if planError.Message != nil {
		definition, err := contextErrorDefinitionFor(planError.Code)
		if err != nil || *planError.Message != definition.message {
			return fmt.Errorf("%w: plan error message", ErrInvalidContextPlan)
		}
	}
	return nil
}

type ContextPlan struct {
	ID                     uint64
	RunID                  uint64
	Profile                *ProfileSnapshot
	PolicyVersion          string
	InputFingerprintSHA256 [sha256.Size]byte
	PlanSHA256             *[sha256.Size]byte
	ModelCapabilitySHA256  [sha256.Size]byte
	APIProtocol            string
	TokenCounterID         string
	Budget                 Budget
	RetrievalOutcome       RetrievalOutcome
	State                  PlanState
	Error                  *PlanError
	Metrics                ContextPlanMetricsV1
	Items                  []ContextPlanItem
}

func (plan ContextPlan) Validate() error {
	if plan.RunID == 0 || !validIdentifier(plan.PolicyVersion, 64) || !validIdentifier(plan.TokenCounterID, 64) {
		return ErrInvalidContextPlan
	}
	if isZeroSHA256(plan.InputFingerprintSHA256) || isZeroSHA256(plan.ModelCapabilitySHA256) {
		return ErrInvalidContextPlan
	}
	if plan.Profile != nil {
		if err := plan.Profile.Validate(); err != nil {
			return err
		}
	}
	if plan.APIProtocol != APIProtocolChatCompletions && plan.APIProtocol != APIProtocolResponses {
		return ErrInvalidContextPlan
	}
	if err := plan.Budget.Validate(); err != nil {
		return err
	}
	if err := plan.RetrievalOutcome.Validate(); err != nil {
		return err
	}
	if err := plan.State.Validate(); err != nil {
		return err
	}
	if err := plan.Metrics.Validate(); err != nil {
		return err
	}

	switch plan.State {
	case PlanReady:
		if plan.PlanSHA256 == nil || isZeroSHA256(*plan.PlanSHA256) || plan.Error != nil || plan.RetrievalOutcome == RetrievalFailed || len(plan.Items) == 0 {
			return ErrInvalidContextPlan
		}
		return validatePlanItems(plan.Items, plan.Budget.KnownInputUpperBound)
	case PlanFailed:
		if plan.PlanSHA256 != nil || plan.RetrievalOutcome != RetrievalFailed || plan.Error == nil || len(plan.Items) != 0 {
			return ErrInvalidContextPlan
		}
		return plan.Error.Validate()
	}
	return ErrInvalidContextPlan
}

type ContextPlanItem struct {
	Ordinal         uint32
	Block           ContextBlock
	Decision        Decision
	ExclusionReason *ExclusionReason
	FusionScore     *FixedScore
	RerankScore     *FixedScore
	CitationKey     *string
}

type ContextBlock struct {
	Kind            BlockKind
	SourceType      string
	SourceRef       string
	SourceSHA256    [sha256.Size]byte
	AtomicGroupKey  string
	Required        bool
	Priority        int32
	TokenUpperBound int64
	ContentSnapshot *string
	Metadata        ContextBlockMetadataV1
}

type ContextPlanMetricsV1 struct {
	Schema                     string  `json:"schema"`
	AuthorizationMS            uint64  `json:"authorization_ms,omitempty"`
	ConversationMS             uint64  `json:"conversation_ms,omitempty"`
	QueryEmbeddingMS           uint64  `json:"query_embedding_ms,omitempty"`
	RetrievalMS                uint64  `json:"retrieval_ms,omitempty"`
	RerankMS                   uint64  `json:"rerank_ms,omitempty"`
	PackingMS                  uint64  `json:"packing_ms,omitempty"`
	CandidateCount             uint32  `json:"candidate_count,omitempty"`
	QueryEmbeddingRequestCount uint32  `json:"query_embedding_request_count,omitempty"`
	RerankRequestCount         uint32  `json:"rerank_request_count,omitempty"`
	QueryInputTokens           *uint64 `json:"query_input_tokens,omitempty"`
	RerankInputTokens          *uint64 `json:"rerank_input_tokens,omitempty"`
}

func (metrics ContextPlanMetricsV1) Validate() error {
	if metrics.Schema != ContextPlanMetricsSchemaV1 {
		return invalidValue("metrics schema", metrics.Schema)
	}
	return nil
}

type ContextBlockMetadataV1 struct {
	Schema    string               `json:"schema"`
	Locator   *ContextLocatorV1    `json:"locator,omitempty"`
	Retrieval *RetrievalBranchesV1 `json:"retrieval,omitempty"`
}

func (metadata ContextBlockMetadataV1) Validate() error {
	if metadata.Schema != ContextBlockMetadataSchemaV1 {
		return invalidValue("block metadata schema", metadata.Schema)
	}
	if metadata.Locator != nil {
		if err := metadata.Locator.Validate(); err != nil {
			return err
		}
	}
	if metadata.Retrieval != nil {
		if err := metadata.Retrieval.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ContextLocatorV1 struct {
	Schema      string   `json:"schema"`
	Kind        string   `json:"kind"`
	Page        *uint32  `json:"page,omitempty"`
	Paragraph   *uint32  `json:"paragraph,omitempty"`
	LineStart   *uint32  `json:"line_start,omitempty"`
	LineEnd     *uint32  `json:"line_end,omitempty"`
	RowStart    *uint32  `json:"row_start,omitempty"`
	RowEnd      *uint32  `json:"row_end,omitempty"`
	Sheet       *string  `json:"sheet,omitempty"`
	CellStart   *string  `json:"cell_start,omitempty"`
	CellEnd     *string  `json:"cell_end,omitempty"`
	HeadingPath []string `json:"heading_path,omitempty"`
}

func (locator ContextLocatorV1) Validate() error {
	if locator.Schema != ContextLocatorSchemaV1 || !validIdentifier(locator.Kind, 32) ||
		invalidOptionalString(locator.Sheet) || invalidOptionalString(locator.CellStart) || invalidOptionalString(locator.CellEnd) {
		return invalidValue("context locator", locator.Kind)
	}
	for _, heading := range locator.HeadingPath {
		if strings.TrimSpace(heading) == "" {
			return invalidValue("context locator heading", heading)
		}
	}
	return nil
}

type RetrievalBranchesV1 struct {
	Schema   string              `json:"schema"`
	Branches []RetrievalBranchV1 `json:"branches"`
}

type RetrievalBranchV1 struct {
	VariantID string     `json:"variant_id"`
	Modality  string     `json:"modality"`
	Rank      uint64     `json:"rank"`
	Score     FixedScore `json:"score"`
}

func (branches RetrievalBranchesV1) Validate() error {
	if branches.Schema != RetrievalBranchesSchemaV1 || len(branches.Branches) == 0 {
		return invalidValue("retrieval branches schema", branches.Schema)
	}
	for _, branch := range branches.Branches {
		if !validIdentifier(branch.VariantID, 64) || (branch.Modality != "dense" && branch.Modality != "sparse") || branch.Rank == 0 {
			return invalidValue("retrieval branch", branch.VariantID)
		}
		if err := branch.Score.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type FixedScore struct{ canonical string }

var fixedScorePattern = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]+))?$`)
var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
var sourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
var citationPattern = regexp.MustCompile(`^C[1-9][0-9]*$`)

func ParseFixedScore(raw string) (FixedScore, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return FixedScore{}, ErrInvalidFixedScore
	}
	match := fixedScorePattern.FindStringSubmatch(raw)
	if match == nil || len(match[3]) > 6 {
		return FixedScore{}, ErrInvalidFixedScore
	}
	integer := strings.TrimLeft(match[2], "0")
	if integer == "" {
		integer = "0"
	}
	if len(integer) > 14 {
		return FixedScore{}, ErrInvalidFixedScore
	}
	fraction := match[3] + strings.Repeat("0", 6-len(match[3]))
	sign := match[1]
	if sign == "+" || (integer == "0" && fraction == "000000") {
		sign = ""
	}
	return FixedScore{canonical: sign + integer + "." + fraction}, nil
}

func FixedScoreFromFloat64(value float64) (FixedScore, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return FixedScore{}, ErrInvalidFixedScore
	}
	return ParseFixedScore(strconv.FormatFloat(value, 'f', 6, 64))
}

func (score FixedScore) Validate() error {
	parsed, err := ParseFixedScore(score.canonical)
	if err != nil || parsed.canonical != score.canonical {
		return ErrInvalidFixedScore
	}
	return nil
}

func (score FixedScore) String() string { return score.canonical }

func (score FixedScore) Compare(other FixedScore) (int, error) {
	if err := score.Validate(); err != nil {
		return 0, err
	}
	if err := other.Validate(); err != nil {
		return 0, err
	}
	left, ok := new(big.Rat).SetString(score.canonical)
	if !ok {
		return 0, ErrInvalidFixedScore
	}
	right, ok := new(big.Rat).SetString(other.canonical)
	if !ok {
		return 0, ErrInvalidFixedScore
	}
	return left.Cmp(right), nil
}

func (score FixedScore) MarshalJSON() ([]byte, error) {
	if err := score.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(score.canonical)
}

func (score *FixedScore) UnmarshalJSON(raw []byte) error {
	if score == nil {
		return ErrInvalidFixedScore
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ErrInvalidFixedScore
	}
	parsed, err := ParseFixedScore(value)
	if err != nil {
		return err
	}
	*score = parsed
	return nil
}

func SHA256FromBytes(raw []byte) ([sha256.Size]byte, error) {
	var value [sha256.Size]byte
	if len(raw) != sha256.Size {
		return value, ErrInvalidSHA256
	}
	copy(value[:], raw)
	return value, nil
}

func validatePlanItems(items []ContextPlanItem, expectedUpperBound int64) error {
	var selectedUpperBound int64
	citations := make(map[string]struct{})
	for index, item := range items {
		if item.Ordinal != uint32(index+1) {
			return ErrInvalidContextPlan
		}
		if err := validatePlanItem(item); err != nil {
			return err
		}
		if item.Decision == DecisionSelected {
			if item.Block.TokenUpperBound > math.MaxInt64-selectedUpperBound {
				return ErrInvalidContextPlan
			}
			selectedUpperBound += item.Block.TokenUpperBound
		}
		if item.CitationKey != nil {
			if _, exists := citations[*item.CitationKey]; exists {
				return ErrInvalidContextPlan
			}
			citations[*item.CitationKey] = struct{}{}
		}
	}
	if selectedUpperBound != expectedUpperBound {
		return ErrInvalidContextPlan
	}
	return nil
}

func validatePlanItem(item ContextPlanItem) error {
	if item.Ordinal == 0 || item.Block.TokenUpperBound < 0 || !sourceTypePattern.MatchString(item.Block.SourceType) ||
		strings.TrimSpace(item.Block.SourceRef) == "" || strings.TrimSpace(item.Block.AtomicGroupKey) == "" ||
		isZeroSHA256(item.Block.SourceSHA256) {
		return ErrInvalidContextPlan
	}
	if err := item.Block.Kind.Validate(); err != nil {
		return err
	}
	if err := item.Block.Metadata.Validate(); err != nil {
		return err
	}
	if err := item.Decision.Validate(); err != nil {
		return err
	}
	if item.FusionScore != nil {
		if err := item.FusionScore.Validate(); err != nil {
			return err
		}
	}
	if item.RerankScore != nil {
		if err := item.RerankScore.Validate(); err != nil {
			return err
		}
	}

	switch item.Decision {
	case DecisionSelected:
		if item.ExclusionReason != nil {
			return ErrInvalidContextPlan
		}
		if item.Block.Kind.isAttachment() {
			if item.Block.ContentSnapshot != nil {
				return ErrInvalidContextPlan
			}
		} else if item.Block.ContentSnapshot == nil || strings.TrimSpace(*item.Block.ContentSnapshot) == "" {
			return ErrInvalidContextPlan
		}
	case DecisionExcluded:
		if item.ExclusionReason == nil || item.ExclusionReason.Validate() != nil || item.Block.ContentSnapshot != nil {
			return ErrInvalidContextPlan
		}
	}

	if item.CitationKey != nil {
		if *item.CitationKey == "" || !citationPattern.MatchString(*item.CitationKey) ||
			item.Decision != DecisionSelected || item.Block.Kind != BlockDocumentEvidence {
			return ErrInvalidContextPlan
		}
	}
	return nil
}

func strictJSONDecode(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == nil {
		return ErrInvalidContextValue
	}
	if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func invalidGeneration(value *uint64) bool { return value != nil && *value == 0 }

func equalGeneration(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func generationAfter(candidate *uint64, previous ...*uint64) bool {
	if candidate == nil {
		return false
	}
	for _, value := range previous {
		if value != nil && *candidate <= *value {
			return false
		}
	}
	return true
}

func invalidProfileIndexError(code *ErrorCode) bool {
	if code == nil {
		return false
	}
	switch *code {
	case ErrCodeProfileUnavailable, ErrCodeIndexFailed, ErrCodeIndexInconsistent:
		return false
	}
	return true
}

func isZeroSHA256(value [sha256.Size]byte) bool {
	return value == [sha256.Size]byte{}
}

func invalidOptionalString(value *string) bool {
	return value != nil && strings.TrimSpace(*value) == ""
}

func validIdentifier(value string, maximum int) bool {
	return len(value) <= maximum && identifierPattern.MatchString(value)
}

func invalidValue(kind, value string) error {
	return fmt.Errorf("%w: %s %q", ErrInvalidContextValue, kind, value)
}
