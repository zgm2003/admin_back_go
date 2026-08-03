package contextengine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEvaluationServicePacksWithoutPersistence(t *testing.T) {
	score, _ := ParseFixedScore("0.900000")
	runner := NewEvaluationService(evaluationPipelineStub{result: EvaluationPipelineResult{
		Outcome: RetrievalHit,
		Budget:  Budget{KnownInputBudget: 10, ToolContinuationInputReserve: 0},
		Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1},
		Groups:  []PackGroup{{Priority: 1, Relevance: &score, StableSourceID: "document:1", Blocks: []PackBlock{{Block: ContextBlock{Kind: BlockDocumentEvidence, SourceType: "document_chunk", SourceRef: "document_chunks:1", SourceSHA256: [32]byte{1}, TokenUpperBound: 4, ContentSnapshot: stringPointer("safe content"), Metadata: ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1, Document: &ContextDocumentEvidenceV1{Title: "doc", DocumentID: 1, DocumentVersionID: 1, ChunkIDs: []uint64{1}, Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph"}}}}}, FusionScore: &score}}}},
	}})
	result, err := runner.RunEvaluation(context.Background(), EvaluationRequest{AgentID: 7, Query: "find"}, EvaluationOptions{Persist: false})
	if err != nil || result.RetrievalOutcome != RetrievalHit || len(result.Selected) != 1 || len(result.Excluded) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Selected[0].Decision != DecisionSelected || result.Selected[0].CitationKey == nil || *result.Selected[0].CitationKey != "C1" {
		t.Fatalf("selected=%+v", result.Selected)
	}
}

func TestEvaluationQueryLimitCountsCharacters(t *testing.T) {
	if !validEvaluationQuery(strings.Repeat("中", 20000)) || validEvaluationQuery(strings.Repeat("中", 20001)) {
		t.Fatal("evaluation query character limit is not 1..20000")
	}
}

type evaluationPipelineStub struct{ result EvaluationPipelineResult }

func (stub evaluationPipelineStub) Evaluate(context.Context, uint64, string) (EvaluationPipelineResult, error) {
	return stub.result, nil
}

func TestEvaluationCorpusIsClosedAndHasDeterministicCategoryCounts(t *testing.T) {
	file, err := os.Open("testdata/evaluation_corpus_v1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cases, err := LoadEvaluationCorpus(file)
	if err != nil {
		t.Fatalf("LoadEvaluationCorpus: %v", err)
	}
	if err := ValidateEvaluationCorpus(cases); err != nil {
		t.Fatalf("ValidateEvaluationCorpus: %v", err)
	}
	counts := map[string]int{}
	for _, item := range cases {
		counts[item.Category]++
	}
	want := map[string]int{"lexical": 20, "semantic": 20, "multi_turn": 10, "no_hit": 5, "cross_scope": 5}
	if !reflect.DeepEqual(counts, want) || len(cases) != 60 {
		t.Fatalf("corpus counts=%v cases=%d, want=%v cases=60", counts, len(cases), want)
	}
}

func TestEvaluationRejectsUnknownFieldsAndInvalidCaseShape(t *testing.T) {
	_, err := LoadEvaluationCorpus(strings.NewReader(`{"id":"x","category":"lexical","query":"q","expected_source_ids":["s"],"denied_source_ids":[],"extra":true}` + "\n"))
	if err == nil {
		t.Fatal("unknown evaluation fields must be rejected")
	}
	if err := ValidateEvaluationCorpus([]EvaluationCase{{ID: "x", Category: "no_hit", Query: "q", ExpectedSourceIDs: []string{"unexpected"}}}); err == nil {
		t.Fatal("no-hit cases must not contain expected sources")
	}
}

func TestEvaluationReportsRetrievalAndCitationThresholdMetrics(t *testing.T) {
	file, err := os.Open("testdata/evaluation_corpus_v1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cases, err := LoadEvaluationCorpus(file)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]EvaluationResult, 0, len(cases))
	for index, item := range cases {
		result := evaluationPackedResult(t, item, index)
		if index == 0 {
			result.RetrievedSourceIDs = append(result.RetrievedSourceIDs, item.ExpectedSourceIDs[0])
		}
		results = append(results, result)
	}
	report, err := Evaluate(cases, results)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if report.CaseCount != 60 || report.Metrics.RecallAt10 != 1 || report.Metrics.MRRAt10 < 0.75 ||
		report.Metrics.NoHitFalsePositiveRate > 0.05 || report.Metrics.CrossScopeLeakage != 0 || report.Metrics.CitationMappingValidity != 1 {
		t.Fatalf("evaluation report=%+v", report)
	}
}

func evaluationPackedResult(t *testing.T, item EvaluationCase, caseIndex int) EvaluationResult {
	t.Helper()
	if len(item.ExpectedSourceIDs) == 0 {
		return EvaluationResult{CaseID: item.ID, RetrievedSourceIDs: []string{}, CitationMappingsValid: true}
	}
	groups := make([]PackGroup, 0, len(item.ExpectedSourceIDs))
	for sourceIndex, sourceID := range item.ExpectedSourceIDs {
		paragraph := uint32(sourceIndex + 1)
		content := "synthetic evidence for " + item.ID
		documentID := uint64(caseIndex*10 + sourceIndex + 1)
		groups = append(groups, PackGroup{
			Priority: 20, SourceOrder: int64(sourceIndex), StableSourceID: sourceID,
			Blocks: []PackBlock{{Block: ContextBlock{
				Kind: BlockDocumentEvidence, SourceType: "document_chunk", SourceRef: sourceID,
				SourceSHA256: sha256.Sum256([]byte(sourceID)), TokenUpperBound: 1, ContentSnapshot: &content,
				Metadata: ContextBlockMetadataV1{
					Schema: ContextBlockMetadataSchemaV1,
					Document: &ContextDocumentEvidenceV1{
						Title: fmt.Sprintf("Synthetic %02d", caseIndex+1), DocumentID: documentID, DocumentVersionID: documentID + 1000,
						ChunkIDs: []uint64{documentID + 2000}, Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}},
					},
				},
			}}},
		})
	}
	packed, appErr := Pack(PackInput{KnownInputBudget: 100, Candidates: groups})
	if appErr != nil {
		t.Fatalf("Pack %s: %v", item.ID, appErr)
	}
	retrieved := make([]string, 0, len(packed.Items))
	validCitations := true
	for _, packedItem := range packed.Items {
		if packedItem.Decision != DecisionSelected {
			continue
		}
		retrieved = append(retrieved, packedItem.Block.SourceRef)
		validCitations = validCitations && packedItem.CitationKey != nil
	}
	return EvaluationResult{CaseID: item.ID, RetrievedSourceIDs: retrieved, CitationMappingsValid: validCitations}
}
