package contextengine

import (
	"regexp"
	"unicode/utf8"
)

const ContextPlanSnapshotRuneLimit = 1000

var messageCitationPattern = regexp.MustCompile(`\[C([1-9][0-9]*)\]`)

type MessageContext struct {
	PlanID      uint64           `json:"plan_id"`
	Outcome     RetrievalOutcome `json:"outcome"`
	Sources     []CitationSource `json:"sources"`
	InvalidKeys []string         `json:"invalid_keys"`
}

type CitationSource struct {
	Key               string           `json:"key"`
	Cited             bool             `json:"cited"`
	Title             string           `json:"title"`
	Locator           ContextLocatorV1 `json:"locator"`
	DocumentID        uint64           `json:"document_id"`
	DocumentVersionID uint64           `json:"document_version_id"`
}

type ContextPlanProjection struct {
	ID               uint64                      `json:"id"`
	Profile          *ContextPlanProfile         `json:"profile"`
	PolicyVersion    string                      `json:"policy_version"`
	APIProtocol      string                      `json:"api_protocol"`
	TokenCounterID   string                      `json:"token_counter_id"`
	RetrievalOutcome RetrievalOutcome            `json:"retrieval_outcome"`
	State            PlanState                   `json:"state"`
	Error            *ContextPlanErrorProjection `json:"error"`
	Budget           Budget                      `json:"budget"`
	Metrics          ContextPlanMetricsV1        `json:"metrics"`
	Items            []ContextPlanItemProjection `json:"items"`
}

type ContextPlanProfile struct {
	ID              uint64  `json:"id"`
	IndexGeneration *uint64 `json:"index_generation"`
}

type ContextPlanErrorProjection struct {
	Stage   string    `json:"stage"`
	Code    ErrorCode `json:"code"`
	Message *string   `json:"message"`
}

type ContextPlanItemProjection struct {
	Ordinal           uint32            `json:"ordinal"`
	Kind              BlockKind         `json:"kind"`
	SourceType        string            `json:"source_type"`
	SourceRef         string            `json:"source_ref"`
	Required          bool              `json:"required"`
	Priority          int32             `json:"priority"`
	TokenUpperBound   int64             `json:"token_upper_bound"`
	Decision          Decision          `json:"decision"`
	ExclusionReason   *ExclusionReason  `json:"exclusion_reason"`
	FusionScore       *string           `json:"fusion_score"`
	RerankScore       *string           `json:"rerank_score"`
	CitationKey       *string           `json:"citation_key"`
	Title             string            `json:"title"`
	Locator           *ContextLocatorV1 `json:"locator"`
	DocumentID        uint64            `json:"document_id"`
	DocumentVersionID uint64            `json:"document_version_id"`
	ContentSnapshot   string            `json:"content_snapshot"`
	ContentTruncated  bool              `json:"content_truncated"`
}

func ProjectMessageContext(content string, plan ContextPlan) (MessageContext, error) {
	if plan.ID == 0 || plan.Validate() != nil {
		return MessageContext{}, ErrInvalidContextPlan
	}
	if plan.RetrievalOutcome == RetrievalDegraded {
		return MessageContext{
			PlanID: plan.ID, Outcome: RetrievalDegraded,
			Sources: make([]CitationSource, 0), InvalidKeys: make([]string, 0),
		}, nil
	}

	sources := make([]CitationSource, 0)
	sourceIndexes := make(map[string]int)
	for _, item := range plan.Items {
		if item.Decision != DecisionSelected || item.Block.Kind != BlockDocumentEvidence || item.CitationKey == nil ||
			item.Block.Metadata.Document == nil || len(item.Block.Metadata.Document.Locators) == 0 {
			continue
		}
		document := item.Block.Metadata.Document
		sourceIndexes[*item.CitationKey] = len(sources)
		sources = append(sources, CitationSource{
			Key: *item.CitationKey, Title: document.Title, Locator: document.Locators[0],
			DocumentID: document.DocumentID, DocumentVersionID: document.DocumentVersionID,
		})
	}

	invalid := make([]string, 0)
	invalidSet := make(map[string]struct{})
	for _, match := range messageCitationPattern.FindAllStringSubmatch(content, -1) {
		key := "C" + match[1]
		if index, ok := sourceIndexes[key]; ok {
			sources[index].Cited = true
			continue
		}
		if _, exists := invalidSet[key]; exists {
			continue
		}
		invalidSet[key] = struct{}{}
		invalid = append(invalid, key)
	}

	return MessageContext{PlanID: plan.ID, Outcome: plan.RetrievalOutcome, Sources: sources, InvalidKeys: invalid}, nil
}

func ProjectContextPlan(plan ContextPlan) (ContextPlanProjection, error) {
	if plan.ID == 0 || plan.Validate() != nil {
		return ContextPlanProjection{}, ErrInvalidContextPlan
	}
	projection := ContextPlanProjection{
		ID: plan.ID, PolicyVersion: plan.PolicyVersion, APIProtocol: plan.APIProtocol,
		TokenCounterID: plan.TokenCounterID, RetrievalOutcome: plan.RetrievalOutcome, State: plan.State,
		Budget: plan.Budget, Metrics: plan.Metrics, Items: make([]ContextPlanItemProjection, 0, len(plan.Items)),
	}
	if plan.Profile != nil {
		projection.Profile = &ContextPlanProfile{ID: plan.Profile.ID, IndexGeneration: cloneUint64(plan.Profile.IndexGeneration)}
	}
	if plan.Error != nil {
		projection.Error = &ContextPlanErrorProjection{Stage: plan.Error.Stage, Code: plan.Error.Code, Message: cloneString(plan.Error.Message)}
	}
	for _, item := range plan.Items {
		projected, err := projectContextPlanItem(item)
		if err != nil {
			return ContextPlanProjection{}, err
		}
		projection.Items = append(projection.Items, projected)
	}
	return projection, nil
}

func projectContextPlanItem(item ContextPlanItem) (ContextPlanItemProjection, error) {
	projection := ContextPlanItemProjection{
		Ordinal: item.Ordinal, Kind: item.Block.Kind, SourceType: item.Block.SourceType, SourceRef: item.Block.SourceRef,
		Required: item.Block.Required, Priority: item.Block.Priority, TokenUpperBound: item.Block.TokenUpperBound,
		Decision: item.Decision, ExclusionReason: cloneExclusionReason(item.ExclusionReason), CitationKey: cloneString(item.CitationKey),
	}
	if item.FusionScore != nil {
		value := item.FusionScore.String()
		projection.FusionScore = &value
	}
	if item.RerankScore != nil {
		value := item.RerankScore.String()
		projection.RerankScore = &value
	}
	if document := item.Block.Metadata.Document; document != nil {
		if len(document.Locators) == 0 {
			return ContextPlanItemProjection{}, ErrInvalidContextPlan
		}
		projection.Title = document.Title
		projection.DocumentID = document.DocumentID
		projection.DocumentVersionID = document.DocumentVersionID
		locator := document.Locators[0]
		projection.Locator = &locator
	}
	if item.Block.ContentSnapshot != nil {
		var err error
		projection.ContentSnapshot, projection.ContentTruncated, err = boundedContextSnapshot(*item.Block.ContentSnapshot)
		if err != nil {
			return ContextPlanItemProjection{}, err
		}
	}
	return projection, nil
}

func boundedContextSnapshot(value string) (string, bool, error) {
	if !utf8.ValidString(value) {
		return "", false, ErrInvalidContextPlan
	}
	if utf8.RuneCountInString(value) <= ContextPlanSnapshotRuneLimit {
		return value, false, nil
	}
	runes := []rune(value)
	return string(runes[:ContextPlanSnapshotRuneLimit]), true, nil
}

func cloneExclusionReason(value *ExclusionReason) *ExclusionReason {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
