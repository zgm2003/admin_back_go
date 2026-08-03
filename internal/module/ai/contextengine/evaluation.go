package contextengine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const evaluationTopK = 10

type EvaluationRequest struct {
	AgentID uint64 `json:"agent_id" binding:"required,gt=0"`
	Query   string `json:"query" binding:"required,min=1,max=20000"`
}

type EvaluationPipelineResult struct {
	Outcome RetrievalOutcome
	Budget  Budget
	Metrics ContextPlanMetricsV1
	Groups  []PackGroup
}

type EvaluationPipeline interface {
	Evaluate(context.Context, uint64, string) (EvaluationPipelineResult, error)
}

type EvaluationService struct{ pipeline EvaluationPipeline }

func NewEvaluationService(pipeline EvaluationPipeline) *EvaluationService {
	if pipeline == nil {
		return nil
	}
	return &EvaluationService{pipeline: pipeline}
}

func (service *EvaluationService) RunEvaluation(ctx context.Context, request EvaluationRequest, options EvaluationOptions) (ContextEvaluationResponse, error) {
	if service == nil || service.pipeline == nil || options.Persist || request.AgentID == 0 || !validEvaluationQuery(request.Query) {
		return ContextEvaluationResponse{}, ErrInvalidContextPlan
	}
	result, err := service.pipeline.Evaluate(ctx, request.AgentID, strings.TrimSpace(request.Query))
	if err != nil {
		return ContextEvaluationResponse{}, err
	}
	if result.Outcome.Validate() != nil || result.Outcome == RetrievalFailed {
		return ContextEvaluationResponse{}, ErrInvalidContextPlan
	}
	if result.Outcome != RetrievalHit {
		if len(result.Groups) != 0 {
			return ContextEvaluationResponse{}, ErrInvalidContextPlan
		}
		return ContextEvaluationResponse{RetrievalOutcome: result.Outcome, Budget: result.Budget, Metrics: result.Metrics, Selected: []EvaluationItemDTO{}, Excluded: []EvaluationItemDTO{}}, nil
	}
	packed, appErr := Pack(PackInput{KnownInputBudget: result.Budget.KnownInputBudget, ToolContinuationInputReserve: result.Budget.ToolContinuationInputReserve, Candidates: result.Groups})
	if appErr != nil {
		return ContextEvaluationResponse{}, appErr
	}
	response := ContextEvaluationResponse{RetrievalOutcome: result.Outcome, Budget: result.Budget, Metrics: result.Metrics, Selected: []EvaluationItemDTO{}, Excluded: []EvaluationItemDTO{}}
	response.Budget.KnownInputUpperBound = packed.KnownInputUpperBound
	for _, item := range packed.Items {
		dto := EvaluationItemDTO{Ordinal: item.Ordinal, Decision: item.Decision, SourceType: item.Block.SourceType, SourceRef: item.Block.SourceRef, CitationKey: cloneString(item.CitationKey), ExclusionReason: clonePointer(item.ExclusionReason), FusionScore: clonePointer(item.FusionScore), RerankScore: clonePointer(item.RerankScore), TokenUpperBound: item.Block.TokenUpperBound, Metadata: cloneContextBlockMetadata(item.Block.Metadata)}
		if item.Decision == DecisionSelected {
			response.Selected = append(response.Selected, dto)
		} else {
			response.Excluded = append(response.Excluded, dto)
		}
	}
	return response, nil
}

func validEvaluationQuery(value string) bool {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && count >= 1 && count <= 20000
}

var evaluationCategoryCounts = map[string]int{
	"lexical": 20, "semantic": 20, "multi_turn": 10, "no_hit": 5, "cross_scope": 5,
}

type EvaluationCase struct {
	ID                string   `json:"id"`
	Category          string   `json:"category"`
	Query             string   `json:"query"`
	ExpectedSourceIDs []string `json:"expected_source_ids"`
	DeniedSourceIDs   []string `json:"denied_source_ids"`
}

type EvaluationResult struct {
	CaseID                string
	RetrievedSourceIDs    []string
	CitationMappingsValid bool
}

type EvaluationMetrics struct {
	RecallAt10              float64 `json:"recall_at_10"`
	MRRAt10                 float64 `json:"mrr_at_10"`
	NoHitFalsePositiveRate  float64 `json:"no_hit_false_positive_rate"`
	CrossScopeLeakage       float64 `json:"cross_scope_leakage"`
	CitationMappingValidity float64 `json:"citation_mapping_validity"`
}

type EvaluationReport struct {
	CaseCount int               `json:"case_count"`
	Metrics   EvaluationMetrics `json:"metrics"`
}

func LoadEvaluationCorpus(reader io.Reader) ([]EvaluationCase, error) {
	if reader == nil {
		return nil, errors.New("evaluation corpus reader is nil")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	cases := make([]EvaluationCase, 0, 60)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		var item EvaluationCase
		if err := decoder.Decode(&item); err != nil {
			return nil, fmt.Errorf("decode evaluation corpus line %d: %w", line, err)
		}
		if err := requireJSONEOF(decoder); err != nil {
			return nil, fmt.Errorf("decode evaluation corpus line %d: %w", line, err)
		}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan evaluation corpus: %w", err)
	}
	return cases, nil
}

func ValidateEvaluationCorpus(cases []EvaluationCase) error {
	if len(cases) != 60 {
		return fmt.Errorf("evaluation corpus has %d cases, want 60", len(cases))
	}
	counts := make(map[string]int, len(evaluationCategoryCounts))
	seenIDs := make(map[string]struct{}, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ID) != item.ID || strings.TrimSpace(item.Query) == "" || strings.TrimSpace(item.Query) != item.Query {
			return fmt.Errorf("invalid evaluation case identity %q", item.ID)
		}
		if _, known := evaluationCategoryCounts[item.Category]; !known {
			return fmt.Errorf("unknown evaluation category %q", item.Category)
		}
		if _, duplicate := seenIDs[item.ID]; duplicate {
			return fmt.Errorf("duplicate evaluation case %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		counts[item.Category]++
		if item.Category == "no_hit" {
			if len(item.ExpectedSourceIDs) != 0 || len(item.DeniedSourceIDs) != 0 {
				return fmt.Errorf("no-hit case %q has source expectations", item.ID)
			}
		} else if len(item.ExpectedSourceIDs) == 0 {
			return fmt.Errorf("evaluation case %q has no expected source", item.ID)
		}
		if item.Category == "cross_scope" && len(item.DeniedSourceIDs) == 0 {
			return fmt.Errorf("cross-scope case %q has no denied source", item.ID)
		}
		if err := validateEvaluationSources(item); err != nil {
			return err
		}
	}
	for category, want := range evaluationCategoryCounts {
		if counts[category] != want {
			return fmt.Errorf("evaluation category %q has %d cases, want %d", category, counts[category], want)
		}
	}
	return nil
}

func Evaluate(cases []EvaluationCase, results []EvaluationResult) (EvaluationReport, error) {
	if err := ValidateEvaluationCorpus(cases); err != nil {
		return EvaluationReport{}, err
	}
	byID := make(map[string]EvaluationResult, len(results))
	for _, result := range results {
		if strings.TrimSpace(result.CaseID) == "" || strings.TrimSpace(result.CaseID) != result.CaseID {
			return EvaluationReport{}, errors.New("invalid evaluation result identity")
		}
		if _, duplicate := byID[result.CaseID]; duplicate {
			return EvaluationReport{}, fmt.Errorf("duplicate evaluation result %q", result.CaseID)
		}
		byID[result.CaseID] = result
	}
	if len(byID) != len(cases) {
		return EvaluationReport{}, fmt.Errorf("evaluation results have %d cases, want %d", len(byID), len(cases))
	}

	var expectedTotal, expectedHits int
	var reciprocalRankTotal float64
	var rankedCaseCount, noHitCount, noHitFalsePositives, crossScopeCount, crossScopeLeaks, validCitations int
	for _, item := range cases {
		result, exists := byID[item.ID]
		if !exists {
			return EvaluationReport{}, fmt.Errorf("missing evaluation result %q", item.ID)
		}
		top := result.RetrievedSourceIDs
		if len(top) > evaluationTopK {
			top = top[:evaluationTopK]
		}
		if result.CitationMappingsValid {
			validCitations++
		}
		if item.Category == "no_hit" {
			noHitCount++
			if len(result.RetrievedSourceIDs) != 0 {
				noHitFalsePositives++
			}
		}
		if item.Category == "cross_scope" {
			crossScopeCount++
			if containsAnySource(result.RetrievedSourceIDs, item.DeniedSourceIDs) {
				crossScopeLeaks++
			}
		}
		if len(item.ExpectedSourceIDs) == 0 {
			continue
		}
		rankedCaseCount++
		expectedTotal += len(item.ExpectedSourceIDs)
		firstRank := 0
		foundExpected := make(map[string]struct{}, len(item.ExpectedSourceIDs))
		for rank, sourceID := range top {
			if containsSource(item.ExpectedSourceIDs, sourceID) {
				if _, duplicate := foundExpected[sourceID]; !duplicate {
					foundExpected[sourceID] = struct{}{}
					expectedHits++
				}
				if firstRank == 0 {
					firstRank = rank + 1
				}
			}
		}
		if firstRank > 0 {
			reciprocalRankTotal += 1 / float64(firstRank)
		}
	}
	metrics := EvaluationMetrics{
		RecallAt10: ratio(expectedHits, expectedTotal), MRRAt10: ratioFloat(reciprocalRankTotal, rankedCaseCount),
		NoHitFalsePositiveRate: ratio(noHitFalsePositives, noHitCount), CrossScopeLeakage: ratio(crossScopeLeaks, crossScopeCount),
		CitationMappingValidity: ratio(validCitations, len(cases)),
	}
	return EvaluationReport{CaseCount: len(cases), Metrics: metrics}, nil
}

func validateEvaluationSources(item EvaluationCase) error {
	seen := make(map[string]struct{}, len(item.ExpectedSourceIDs)+len(item.DeniedSourceIDs))
	for _, sourceID := range append(append([]string(nil), item.ExpectedSourceIDs...), item.DeniedSourceIDs...) {
		if strings.TrimSpace(sourceID) == "" || strings.TrimSpace(sourceID) != sourceID {
			return fmt.Errorf("evaluation case %q has invalid source", item.ID)
		}
		if _, duplicate := seen[sourceID]; duplicate {
			return fmt.Errorf("evaluation case %q repeats source %q", item.ID, sourceID)
		}
		seen[sourceID] = struct{}{}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func containsAnySource(actual, denied []string) bool {
	for _, sourceID := range actual {
		if containsSource(denied, sourceID) {
			return true
		}
	}
	return false
}

func containsSource(sources []string, target string) bool {
	for _, sourceID := range sources {
		if sourceID == target {
			return true
		}
	}
	return false
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / float64(denominator)
}
