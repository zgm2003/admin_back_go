package qdrant

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/contextindex"

	qdrantapi "github.com/qdrant/go-client/qdrant"
)

func TestQueryBatchUsesOfficialRRFAndReturnsOnlyFusionCandidates(t *testing.T) {
	point := testDocumentPoint(t, contextindex.SparseVector{Indices: []uint32{9}, Values: []float32{1}})
	api := &fakeAPI{queryResults: []*qdrantapi.BatchResult{
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.9)}},
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.8)}},
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.7)}},
	}}
	filter, err := contextindex.NewScopeFilter(7, 3, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	denseMinScore := 0.25
	result, err := newClient(api).QueryBatch(context.Background(), contextindex.QueryBatchInput{
		Collection: "admin_context_profile_7_g3", Filter: filter, TopN: 20,
		DenseMinScore: &denseMinScore,
		Variants: []contextindex.QueryVariantVector{{
			VariantID: "current", QuerySHA256: [32]byte{1}, Dense: []float32{1, 0, 0, 0},
			Sparse: contextindex.SparseVector{Indices: []uint32{9}, Values: []float32{1}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.queried.GetQueryPoints()) != 3 || api.queried.GetQueryPoints()[2].GetQuery().GetFusion() != qdrantapi.Fusion_RRF {
		t.Fatalf("unexpected QueryBatch shape: %+v", api.queried)
	}
	if api.queried.GetQueryPoints()[0].ScoreThreshold == nil || *api.queried.GetQueryPoints()[0].ScoreThreshold != float32(denseMinScore) || api.queried.GetQueryPoints()[1].ScoreThreshold != nil {
		t.Fatal("dense threshold must apply only to the dense branch")
	}
	if len(result.Fusion) != 1 || len(result.Branches) != 2 || result.Fusion[0].Point != point.Metadata.Ref {
		t.Fatalf("result=%+v", result)
	}
}
