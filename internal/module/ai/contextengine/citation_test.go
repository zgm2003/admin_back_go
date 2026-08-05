package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCitationProjectionPreservesPlanOrderAndClassifiesKeys(t *testing.T) {
	plan := citationTestPlan(t)

	projection, err := ProjectMessageContext("依据 [C2]、[C1]、[C1] 和 [C99] [C99]。[C0] [C-1] [C01] 不是引用。", plan)
	if err != nil {
		t.Fatalf("ProjectMessageContext: %v", err)
	}
	if projection.PlanID != plan.ID || projection.Outcome != RetrievalHit {
		t.Fatalf("projection header=%+v", projection)
	}
	if len(projection.Sources) != 2 {
		t.Fatalf("sources=%+v", projection.Sources)
	}
	if projection.Sources[0].Key != "C1" || projection.Sources[0].Title != "退款规则" || !projection.Sources[0].Cited ||
		projection.Sources[1].Key != "C2" || projection.Sources[1].Title != "到账规则" || !projection.Sources[1].Cited {
		t.Fatalf("sources must remain in Plan order: %+v", projection.Sources)
	}
	if !reflect.DeepEqual(projection.InvalidKeys, []string{"C99"}) {
		t.Fatalf("invalid keys=%v", projection.InvalidKeys)
	}
}

func TestCitationProjectionKeepsSelectedUnmentionedSources(t *testing.T) {
	projection, err := ProjectMessageContext("只引用第一条 [C1]。", citationTestPlan(t))
	if err != nil {
		t.Fatalf("ProjectMessageContext: %v", err)
	}
	if len(projection.Sources) != 2 || !projection.Sources[0].Cited || projection.Sources[1].Cited {
		t.Fatalf("sources=%+v", projection.Sources)
	}
	if len(projection.InvalidKeys) != 0 {
		t.Fatalf("invalid keys=%v", projection.InvalidKeys)
	}
}

func TestDegradedPlanProjectsNoCitationSource(t *testing.T) {
	plan := degradedReadyPlan(t)
	plan.ID = 71
	projection, err := ProjectMessageContext("provider violated the instruction [C1]", plan)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Outcome != RetrievalDegraded || len(projection.Sources) != 0 || len(projection.InvalidKeys) != 0 {
		t.Fatalf("projection=%+v", projection)
	}
}

func TestContextPlanProjectionIsClosedOrderedAndBoundsSnapshots(t *testing.T) {
	plan := citationTestPlan(t)
	longSnapshot := strings.Repeat("界", ContextPlanSnapshotRuneLimit+10)
	plan.Items[0].Block.ContentSnapshot = &longSnapshot
	plan.Budget.KnownInputUpperBound += 10
	plan.Items[0].Block.TokenUpperBound += 10

	projection, err := ProjectContextPlan(plan)
	if err != nil {
		t.Fatalf("ProjectContextPlan: %v", err)
	}
	if projection.ID != plan.ID || projection.RetrievalOutcome != RetrievalHit || projection.Budget != plan.Budget || projection.Metrics != plan.Metrics {
		t.Fatalf("projection header=%+v", projection)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(payload, &contract); err != nil {
		t.Fatal(err)
	}
	if _, exists := contract["retrieval_outcome"]; !exists || contract["outcome"] != nil {
		t.Fatalf("Run Context Plan outcome contract=%s", payload)
	}
	if len(projection.Items) != len(plan.Items) || projection.Items[0].Ordinal != 1 || projection.Items[1].Ordinal != 2 {
		t.Fatalf("items lost Plan order: %+v", projection.Items)
	}
	if got := []rune(projection.Items[0].ContentSnapshot); len(got) != ContextPlanSnapshotRuneLimit {
		t.Fatalf("bounded snapshot runes=%d", len(got))
	}
	if projection.Items[0].Locator == nil || projection.Items[0].DocumentID != 10 || projection.Items[0].DocumentVersionID != 11 {
		t.Fatalf("safe document facts=%+v", projection.Items[0])
	}
	if projection.Items[2].ContentSnapshot != `{"name":"lookup"}` || projection.Items[2].CitationKey != nil {
		t.Fatalf("non-document projection=%+v", projection.Items[2])
	}
}

func TestContextPlanProjectionRejectsInvalidSnapshot(t *testing.T) {
	plan := citationTestPlan(t)
	invalid := string([]byte{0xff})
	plan.Items[0].Block.ContentSnapshot = &invalid

	if _, err := ProjectContextPlan(plan); err == nil {
		t.Fatal("invalid persisted UTF-8 must not be hidden as an empty snapshot")
	}
}

func citationTestPlan(t *testing.T) ContextPlan {
	t.Helper()
	plan := validReadyPlan()
	plan.ID = 71
	plan.RetrievalOutcome = RetrievalHit
	plan.Budget.KnownInputUpperBound = 33
	plan.Items = []ContextPlanItem{
		citationDocumentItem(1, "C1", "退款规则", 10, 11, 20, 1, "退款会在三个工作日内到账。", 11),
		citationDocumentItem(2, "C2", "到账规则", 12, 13, 21, 2, "到账后会发送通知。", 12),
		{
			Ordinal: 3,
			Block: ContextBlock{
				Kind: BlockToolDefinition, SourceType: "tool", SourceRef: "tool:9",
				SourceSHA256: sha256.Sum256([]byte("tool:9")), AtomicGroupKey: "tool:9",
				Required: true, Priority: 3, TokenUpperBound: 10,
				ContentSnapshot: stringPointer(`{"name":"lookup"}`), Metadata: emptyBlockMetadata(),
			},
			Decision: DecisionSelected,
		},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("citation test Plan invalid: %v", err)
	}
	return plan
}

func citationDocumentItem(ordinal uint32, key, title string, documentID, versionID, chunkID uint64, paragraph uint32, content string, tokens int64) ContextPlanItem {
	locator := ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}
	return ContextPlanItem{
		Ordinal: ordinal,
		Block: ContextBlock{
			Kind: BlockDocumentEvidence, SourceType: "document_chunk", SourceRef: "chunk:" + key,
			SourceSHA256: sha256.Sum256([]byte("chunk:" + key)), AtomicGroupKey: "document:" + key,
			Priority: 20, TokenUpperBound: tokens, ContentSnapshot: &content,
			Metadata: ContextBlockMetadataV1{
				Schema: ContextBlockMetadataSchemaV1,
				Document: &ContextDocumentEvidenceV1{
					Title: title, DocumentID: documentID, DocumentVersionID: versionID,
					ChunkIDs: []uint64{chunkID}, Locators: []ContextLocatorV1{locator},
				},
			},
		},
		Decision: DecisionSelected, CitationKey: &key,
	}
}
