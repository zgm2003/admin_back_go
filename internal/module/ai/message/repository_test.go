package aimessage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/enum"
)

type messageContextRepository struct {
	*fakeRepository
	plans    map[uint64]contextengine.ContextPlan
	planRuns []uint64
}

func (repository *messageContextRepository) ContextPlans(_ context.Context, runIDs []uint64) (map[uint64]messageContextPlan, error) {
	repository.planRuns = append(repository.planRuns, runIDs...)
	result := make(map[uint64]messageContextPlan, len(runIDs))
	for _, runID := range runIDs {
		if plan, ok := repository.plans[runID]; ok {
			items := make([]messageContextPlanItem, 0, len(plan.Items))
			for _, item := range plan.Items {
				metadata, _ := json.Marshal(item.Block.Metadata)
				items = append(items, messageContextPlanItem{Decision: string(item.Decision), Kind: string(item.Block.Kind), CitationKey: item.CitationKey, MetadataJSON: string(metadata)})
			}
			result[runID] = messageContextPlan{ID: plan.ID, RunID: plan.RunID, RetrievalOutcome: string(plan.RetrievalOutcome), State: string(plan.State), Items: items}
		}
	}
	return result, nil
}

func TestMessageCitationRefreshProjectsCompletedAndStoppedPersistedReplies(t *testing.T) {
	completed, stopped, pending := DeliveryStateCompleted, DeliveryStateStopped, "pending"
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	run501, run502, run503, run504 := int64(501), int64(502), int64(503), int64(504)
	base := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []MessageProjection{
		{Message: Message{ID: 14, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "历史回答", DeliveryState: &completed, CreatedAt: now, UpdatedAt: now}, RunID: &run504},
		{Message: Message{ID: 13, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "生成中", DeliveryState: &pending, CreatedAt: now, UpdatedAt: now}, RunID: &run503},
		{Message: Message{ID: 12, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "停止前引用 [C1]", DeliveryState: &stopped, CreatedAt: now, UpdatedAt: now}, RunID: &run502},
		{Message: Message{ID: 11, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "完成引用 [C1] 和未知 [C7]", DeliveryState: &completed, CreatedAt: now, UpdatedAt: now}, RunID: &run501},
		{Message: Message{ID: 10, ConversationID: 3, Role: enum.AIMessageRoleUser, Content: "问题", CreatedAt: now, UpdatedAt: now}},
	}}
	repository := &messageContextRepository{fakeRepository: base, plans: map[uint64]contextengine.ContextPlan{
		501: messageProjectionPlan(71, 501),
		502: messageProjectionPlan(72, 502),
	}}
	service := NewService(repository)

	first, appErr := service.List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr != nil {
		t.Fatalf("first refresh: %v", appErr)
	}
	second, appErr := service.List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr != nil {
		t.Fatalf("second refresh: %v", appErr)
	}
	if !reflect.DeepEqual(first.List, second.List) {
		t.Fatalf("refresh changed persisted projection:\nfirst=%+v\nsecond=%+v", first.List, second.List)
	}
	if len(first.List) != 5 || first.List[0].Context != nil || first.List[3].Context != nil || first.List[4].Context != nil {
		t.Fatalf("user, pending and historical rows must not invent Context: %+v", first.List)
	}
	completedContext, stoppedContext := first.List[1].Context, first.List[2].Context
	if completedContext == nil || len(completedContext.Sources) != 1 || !completedContext.Sources[0].Cited || !reflect.DeepEqual(completedContext.InvalidKeys, []string{"C7"}) {
		t.Fatalf("completed Context=%+v", completedContext)
	}
	if stoppedContext == nil || len(stoppedContext.Sources) != 1 || !stoppedContext.Sources[0].Cited || len(stoppedContext.InvalidKeys) != 0 {
		t.Fatalf("stopped Context=%+v", stoppedContext)
	}
	if !reflect.DeepEqual(repository.planRuns, []uint64{501, 502, 504, 501, 502, 504}) {
		t.Fatalf("plan run lookups=%v", repository.planRuns)
	}
}

func TestMessageContextProjectionSerializesEmptyCollectionsAsArrays(t *testing.T) {
	projection, err := projectMessageContext("", messageContextPlan{
		ID: 2, RunID: 29, RetrievalOutcome: "skipped", State: "ready",
	})
	if err != nil {
		t.Fatalf("project skipped context: %v", err)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal skipped context: %v", err)
	}
	const want = `{"plan_id":2,"outcome":"skipped","sources":[],"invalid_keys":[]}`
	if string(payload) != want {
		t.Fatalf("skipped context JSON=%s, want %s", payload, want)
	}
}

func messageProjectionPlan(id, runID uint64) contextengine.ContextPlan {
	planHash := sha256.Sum256([]byte("plan"))
	paragraph := uint32(3)
	content := "退款会在三个工作日内到账。"
	citation := "C1"
	return contextengine.ContextPlan{
		ID: id, RunID: runID, PolicyVersion: "context_policy_v1",
		InputFingerprintSHA256: sha256.Sum256([]byte("input")), PlanSHA256: &planHash,
		ModelCapabilitySHA256: sha256.Sum256([]byte("model")), APIProtocol: contextengine.APIProtocolChatCompletions,
		TokenCounterID: "test_counter_v1",
		Budget: contextengine.Budget{
			ContextWindowTokens: 100, EffectiveOutputTokens: 20, ProviderProtocolUpperBound: 10,
			ToolContinuationInputReserve: 5, PolicySafetyMargin: 10, KnownInputBudget: 60,
			KnownInputUpperBound: 20, Proof: contextengine.BudgetExact,
		},
		RetrievalOutcome: contextengine.RetrievalHit, State: contextengine.PlanReady,
		Metrics: contextengine.ContextPlanMetricsV1{Schema: contextengine.ContextPlanMetricsSchemaV1},
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
