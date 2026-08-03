package contextengine

import (
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
)

func TestQueryVariantEmptyAndContextualSparseReuse(t *testing.T) {
	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	variants, err := BuildQueryVariants("   ", nil, counter, 1024)
	if err != nil || len(variants) != 0 {
		t.Fatalf("empty variants=%+v err=%v", variants, err)
	}
	turn := testConversationTurn()
	variants, err = BuildQueryVariants("refund", &turn, counter, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 || variants[1].Sparse.Empty() {
		t.Fatalf("variants=%+v", variants)
	}
	want, err := EncodeSparse(variants[1].Text)
	if err != nil || len(want.Indices) != len(variants[1].Sparse.Indices) {
		t.Fatalf("shared sparse encoder was not reused: err=%v", err)
	}
}

func TestQueryBatchCandidatesUseFusionOrderAndBranchEvidence(t *testing.T) {
	ref := contextindex.PointRef{ID: uuid.MustParse("80000000-0000-8000-8000-000000000001"), ProfileID: 1, IndexGeneration: 1, SourceKind: contextindex.SourceKindDocumentChunk, SourceID: 1, SourceSHA256: [32]byte{1}}
	result := contextindex.QueryBatchResult{
		Fusion:   []contextindex.QueryFusionHit{{Point: ref, Rank: 1, Score: 0.9}},
		Branches: []contextindex.QueryBranchHit{{Point: ref, VariantID: "current", Modality: contextindex.QueryModalityDense, Rank: 1, Score: 0.8}},
	}
	candidates, err := CandidatesFromQueryBatch(result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Point != ref || len(candidates[0].Branches.Branches) != 1 {
		t.Fatalf("candidates=%+v", candidates)
	}
}
