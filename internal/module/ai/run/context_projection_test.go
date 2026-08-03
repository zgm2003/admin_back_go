package airun

import (
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	"admin_back_go/internal/module/ai/contextengine"
)

func TestContextPlanProjectionRefreshUsesPersistedPlan(t *testing.T) {
	plan := runProjectionPlan()
	repository := &fakeRepository{run: &RunDetailRow{ID: 501}, contextPlan: &plan}
	service := NewService(repository)

	first, appErr := service.Detail(context.Background(), 501)
	if appErr != nil {
		t.Fatalf("first detail: %v", appErr)
	}
	second, appErr := service.Detail(context.Background(), 501)
	if appErr != nil {
		t.Fatalf("second detail: %v", appErr)
	}
	if first.ContextPlan == nil || !reflect.DeepEqual(first.ContextPlan, second.ContextPlan) {
		t.Fatalf("refresh changed Context Plan: first=%+v second=%+v", first.ContextPlan, second.ContextPlan)
	}
	if first.ContextPlan.ID != 71 || first.ContextPlan.Budget.KnownInputUpperBound != 20 ||
		first.ContextPlan.Metrics.RetrievalMS != 8 || len(first.ContextPlan.Items) != 1 ||
		first.ContextPlan.Items[0].CitationKey == nil || *first.ContextPlan.Items[0].CitationKey != "C1" {
		t.Fatalf("Context Plan=%+v", first.ContextPlan)
	}
	if !reflect.DeepEqual(repository.contextPlanRuns, []int64{501, 501}) {
		t.Fatalf("Context Plan run lookups=%v", repository.contextPlanRuns)
	}
}

func TestContextPlanProjectionKeepsHistoricalRunNullable(t *testing.T) {
	result, appErr := NewService(&fakeRepository{run: &RunDetailRow{ID: 44}}).Detail(context.Background(), 44)
	if appErr != nil {
		t.Fatalf("Detail: %v", appErr)
	}
	if result.ContextPlan != nil {
		t.Fatalf("historical Context Plan=%+v", result.ContextPlan)
	}
}

func runProjectionPlan() contextengine.ContextPlan {
	planHash := sha256.Sum256([]byte("plan"))
	paragraph := uint32(3)
	content := "退款会在三个工作日内到账。"
	citation := "C1"
	return contextengine.ContextPlan{
		ID: 71, RunID: 501, PolicyVersion: "context_policy_v1",
		InputFingerprintSHA256: sha256.Sum256([]byte("input")), PlanSHA256: &planHash,
		ModelCapabilitySHA256: sha256.Sum256([]byte("model")), APIProtocol: contextengine.APIProtocolChatCompletions,
		TokenCounterID: "test_counter_v1",
		Budget: contextengine.Budget{
			ContextWindowTokens: 100, EffectiveOutputTokens: 20, ProviderProtocolUpperBound: 10,
			ToolContinuationInputReserve: 5, PolicySafetyMargin: 10, KnownInputBudget: 60,
			KnownInputUpperBound: 20, Proof: contextengine.BudgetExact,
		},
		RetrievalOutcome: contextengine.RetrievalHit, State: contextengine.PlanReady,
		Metrics: contextengine.ContextPlanMetricsV1{Schema: contextengine.ContextPlanMetricsSchemaV1, RetrievalMS: 8, CandidateCount: 3},
		Items: []contextengine.ContextPlanItem{{
			Ordinal: 1,
			Block: contextengine.ContextBlock{
				Kind: contextengine.BlockDocumentEvidence, SourceType: "document_chunk", SourceRef: "chunk:20",
				SourceSHA256: sha256.Sum256([]byte("chunk")), AtomicGroupKey: "document:10", Priority: 20,
				TokenUpperBound: 20, ContentSnapshot: &content,
				Metadata: contextengine.ContextBlockMetadataV1{
					Schema: contextengine.ContextBlockMetadataSchemaV1,
					Document: &contextengine.ContextDocumentEvidenceV1{
						Title: "退款规则", DocumentID: 10, DocumentVersionID: 11, ChunkIDs: []uint64{20},
						Locators: []contextengine.ContextLocatorV1{{Schema: contextengine.ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}},
					},
				},
			},
			Decision: contextengine.DecisionSelected, CitationKey: &citation,
		}},
	}
}
