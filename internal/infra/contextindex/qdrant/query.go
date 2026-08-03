package qdrant

import (
	"context"
	"fmt"

	"admin_back_go/internal/infra/contextindex"

	qdrantapi "github.com/qdrant/go-client/qdrant"
)

func (client *Client) QueryBatch(ctx context.Context, input contextindex.QueryBatchInput) (contextindex.QueryBatchResult, error) {
	if err := input.Validate(); err != nil {
		return contextindex.QueryBatchResult{}, err
	}
	filter := encodeScopeFilter(input.Filter)
	queryPoints := make([]*qdrantapi.QueryPoints, 0, len(input.Variants)*2+1)
	prefetch := make([]*qdrantapi.PrefetchQuery, 0, len(input.Variants)*2)
	type branchSpec struct {
		variantID string
		modality  contextindex.QueryModality
	}
	branches := make([]branchSpec, 0, len(input.Variants)*2)
	for _, variant := range input.Variants {
		dense := &qdrantapi.PrefetchQuery{
			Query: qdrantapi.NewQueryDense(variant.Dense), Using: stringPointer(denseVectorName),
			Filter: filter, Limit: &input.TopN,
		}
		if input.DenseMinScore != nil {
			threshold := float32(*input.DenseMinScore)
			dense.ScoreThreshold = &threshold
		}
		prefetch = append(prefetch, dense)
		queryPoints = append(queryPoints, independentQuery(input.Collection, dense))
		branches = append(branches, branchSpec{variantID: variant.VariantID, modality: contextindex.QueryModalityDense})

		if !variant.Sparse.Empty() {
			sparse := &qdrantapi.PrefetchQuery{
				Query: qdrantapi.NewQuerySparse(variant.Sparse.Indices, variant.Sparse.Values),
				Using: stringPointer(sparseVectorName), Filter: filter, Limit: &input.TopN,
			}
			prefetch = append(prefetch, sparse)
			queryPoints = append(queryPoints, independentQuery(input.Collection, sparse))
			branches = append(branches, branchSpec{variantID: variant.VariantID, modality: contextindex.QueryModalitySparse})
		}
	}
	k := rrfK
	queryPoints = append(queryPoints, &qdrantapi.QueryPoints{
		CollectionName: input.Collection, Prefetch: prefetch,
		Query: qdrantapi.NewQueryRRF(&qdrantapi.Rrf{K: &k}), Filter: filter,
		Limit: &input.TopN, WithPayload: pointPayloadSelector(),
	})
	batch, err := client.api.QueryBatch(ctx, &qdrantapi.QueryBatchPoints{CollectionName: input.Collection, QueryPoints: queryPoints})
	if err != nil {
		return contextindex.QueryBatchResult{}, fmt.Errorf("query Qdrant collection %q: %w", input.Collection, err)
	}
	if len(batch) != len(branches)+1 {
		return contextindex.QueryBatchResult{}, fmt.Errorf("Qdrant batch returned %d results, want %d", len(batch), len(branches)+1)
	}
	result := contextindex.QueryBatchResult{}
	for index, branch := range branches {
		points, decodeErr := decodeScoredPoints(batch[index].GetResult())
		if decodeErr != nil {
			return contextindex.QueryBatchResult{}, fmt.Errorf("decode Qdrant branch %d: %w", index, decodeErr)
		}
		for rank, hit := range points {
			result.Branches = append(result.Branches, contextindex.QueryBranchHit{
				Point: hit.Metadata.Ref, VariantID: branch.variantID, Modality: branch.modality,
				Rank: uint64(rank + 1), Score: float64(hit.Score),
			})
		}
	}
	fused, err := decodeScoredPoints(batch[len(batch)-1].GetResult())
	if err != nil {
		return contextindex.QueryBatchResult{}, fmt.Errorf("decode Qdrant RRF result: %w", err)
	}
	result.Fusion = make([]contextindex.QueryFusionHit, len(fused))
	for rank, hit := range fused {
		result.Fusion[rank] = contextindex.QueryFusionHit{Point: hit.Metadata.Ref, Rank: uint64(rank + 1), Score: float64(hit.Score)}
	}
	if err := result.Validate(); err != nil {
		return contextindex.QueryBatchResult{}, fmt.Errorf("validate Qdrant QueryBatch result: %w", err)
	}
	return result, nil
}
