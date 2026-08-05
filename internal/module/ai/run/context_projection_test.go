package airun

import (
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
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

func TestContextPlanProjectionAddsDegradedDiagnosticWithoutChangingRunTruth(t *testing.T) {
	plan := degradedRunProjectionPlan(t)
	row := &RunDetailRow{
		ID: 501, Status: "success", DiagnosticCodes: []string{"ai.attachment.current_unavailable"},
		BillingStatus: "unbilled", BillingReason: "legacy_unpriced",
	}
	result, appErr := NewService(&fakeRepository{run: row, contextPlan: &plan}).Detail(context.Background(), 501)
	if appErr != nil {
		t.Fatalf("Detail: %v", appErr)
	}
	if result.ContextPlan == nil || result.ContextPlan.RetrievalOutcome != contextengine.RetrievalDegraded ||
		result.ContextPlan.State != contextengine.PlanReady || result.ContextPlan.Error == nil ||
		result.ContextPlan.Error.Stage != string(contextengine.EnhancementStageEmbedding) ||
		result.ContextPlan.Error.Code != contextengine.ErrCodeEmbeddingFailed {
		t.Fatalf("degraded Context Plan=%+v", result.ContextPlan)
	}
	if !reflect.DeepEqual(result.DiagnosticCodes, []string{"ai.attachment.current_unavailable", string(contextengine.ErrCodeEmbeddingFailed)}) {
		t.Fatalf("diagnostic codes=%v", result.DiagnosticCodes)
	}
	if result.Status != "success" || result.ErrorCode != "" || result.BillingStatus != "unbilled" || result.BillingReason != "legacy_unpriced" {
		t.Fatalf("degradation changed Run truth: status=%q error=%q billing=%q/%q", result.Status, result.ErrorCode, result.BillingStatus, result.BillingReason)
	}
}

func TestContextPlanProjectionDoesNotDuplicateExistingDegradedDiagnostic(t *testing.T) {
	plan := degradedRunProjectionPlan(t)
	row := &RunDetailRow{
		ID: 501, DiagnosticCodes: []string{string(contextengine.ErrCodeEmbeddingFailed), "ai.attachment.current_unavailable"},
		BillingStatus: "unbilled", BillingReason: "legacy_unpriced",
	}
	result, appErr := NewService(&fakeRepository{run: row, contextPlan: &plan}).Detail(context.Background(), 501)
	if appErr != nil {
		t.Fatalf("Detail: %v", appErr)
	}
	if !reflect.DeepEqual(result.DiagnosticCodes, row.DiagnosticCodes) {
		t.Fatalf("diagnostic codes=%v want=%v", result.DiagnosticCodes, row.DiagnosticCodes)
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

func degradedRunProjectionPlan(t *testing.T) contextengine.ContextPlan {
	t.Helper()
	plan := runProjectionPlan()
	diagnostic, err := contextengine.NewPlanError(string(contextengine.EnhancementStageEmbedding), contextengine.ErrCodeEmbeddingFailed)
	if err != nil {
		t.Fatal(err)
	}
	content := "hello"
	plan.RetrievalOutcome = contextengine.RetrievalDegraded
	plan.Error = &diagnostic
	plan.TokenCounterID = infraai.TokenCounterUTF8BytesV1
	plan.Budget.Proof = contextengine.BudgetConservative
	plan.Items = []contextengine.ContextPlanItem{{
		Ordinal: 1,
		Block: contextengine.ContextBlock{
			Kind: contextengine.BlockCurrentUserMessage, SourceType: "message", SourceRef: "message:15",
			SourceSHA256: sha256.Sum256([]byte(content)), AtomicGroupKey: "turn:15", Required: true,
			Priority: 1, TokenUpperBound: 20, ContentSnapshot: &content,
			Metadata: contextengine.ContextBlockMetadataV1{Schema: contextengine.ContextBlockMetadataSchemaV1},
		},
		Decision: contextengine.DecisionSelected,
	}}
	plan.PlanSHA256 = nil
	planHash, err := contextengine.HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &planHash
	if err := plan.Validate(); err != nil {
		t.Fatalf("degraded fixture is invalid: %v", err)
	}
	return plan
}
