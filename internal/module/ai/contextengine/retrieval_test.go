package contextengine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
)

func TestNormalizeCandidatesDeduplicatesContentAndMergesAdjacentChunks(t *testing.T) {
	first := verifiedDocumentCandidate(t, 1, 7, 0, "same", "first")
	duplicate := verifiedDocumentCandidate(t, 2, 7, 4, "same", "duplicate")
	second := verifiedDocumentCandidate(t, 3, 7, 1, "other", "second")

	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	result, excluded, err := NormalizeVerifiedCandidates([]VerifiedCandidate{first, duplicate, second}, 100, counter)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || len(result[0].ChunkIDs) != 2 || result[0].Content != "first\n\nsecond" {
		t.Fatalf("result=%+v", result)
	}
	if len(excluded) != 1 || excluded[0].Reason != ExclusionDuplicateContent {
		t.Fatalf("excluded=%+v", excluded)
	}
}

func TestApplyRerankIsDisabledWithoutClientAndStrictWhenConfigured(t *testing.T) {
	candidates := []VerifiedCandidate{
		verifiedDocumentCandidate(t, 1, 7, 0, "a", "first"),
		verifiedDocumentCandidate(t, 2, 8, 0, "b", "second"),
	}
	unchanged, err := ApplyRerank(context.Background(), "refund", candidates, nil, nil)
	if err != nil || unchanged[0].Point.SourceID != candidates[0].Point.SourceID {
		t.Fatalf("disabled result=%+v err=%v", unchanged, err)
	}
	threshold, _ := ParseFixedScore("0.500000")
	client := &fakeRerankClient{result: infraai.RerankResult{ModelID: "rerank-v1", Scores: []infraai.RerankScore{{CandidateID: candidates[1].CandidateID(), Score: 0.9}, {CandidateID: candidates[0].CandidateID(), Score: 0.4}}}}
	reranked, err := ApplyRerank(context.Background(), "refund", candidates, client, &threshold)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 1 || reranked[0].Point.SourceID != candidates[1].Point.SourceID || reranked[0].RerankScore == nil {
		t.Fatalf("reranked=%+v", reranked)
	}
}

func TestRetrieveUsesEmbeddingQueryBatchAuthorityAndNormalization(t *testing.T) {
	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	filter, err := contextindex.NewScopeFilter(1, 1, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified := verifiedDocumentCandidate(t, 1, 7, 0, "a", "first")
	result, err := Retrieve(context.Background(), RetrievalInput{
		Collection: "admin_context_profile_1_g1", Filter: filter,
		Variants: []QueryVariant{{VariantID: "current", Text: "refund", QuerySHA256: hashText("query"), Sparse: contextindex.SparseVector{Indices: []uint32{1}, Values: []float32{1}}}},
		TopN:     20, Authority: CandidateAuthoritySnapshot{ProfileID: 1, IndexGeneration: 1, AgentID: 3, UserID: 4, ConversationID: 5, Platform: "admin"},
		MaxMergedTokens: 100, TokenCounter: counter,
	}, RetrievalDependencies{
		Embedding: fakeEmbeddingClient{result: infraai.EmbeddingResult{ModelID: "embed-v1", Vectors: [][]float32{{1, 0}}}},
		Querier: fakeQuerier{result: contextindex.QueryBatchResult{
			Fusion:   []contextindex.QueryFusionHit{{Point: verified.Point, Rank: 1, Score: 0.9}},
			Branches: []contextindex.QueryBranchHit{{Point: verified.Point, VariantID: "current", Modality: contextindex.QueryModalityDense, Rank: 1, Score: 0.8}},
		}},
		Authority: fakeCandidateAuthority{result: CandidateVerification{Authorized: []VerifiedCandidate{verified}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != RetrievalHit || len(result.Candidates) != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRetrievalClassificationAtOwningBoundaries(t *testing.T) {
	input, dependencies := retrievalClassificationFixture(t)

	t.Run("embedding", func(t *testing.T) {
		cause := errors.New("embedding provider unavailable")
		current := dependencies
		current.Embedding = fakeEmbeddingClient{err: cause}
		_, err := Retrieve(t.Context(), input, current)
		assertEnhancementFailure(t, err, EnhancementStageEmbedding, ErrCodeEmbeddingFailed)
		if !errors.Is(err, cause) {
			t.Fatalf("embedding cause was lost: %v", err)
		}
	})

	t.Run("retrieval", func(t *testing.T) {
		cause := errors.New("qdrant unavailable")
		current := dependencies
		current.Querier = fakeQuerier{err: cause}
		_, err := Retrieve(t.Context(), input, current)
		assertEnhancementFailure(t, err, EnhancementStageRetrieval, ErrCodeRetrievalFailed)
		if !errors.Is(err, cause) {
			t.Fatalf("qdrant cause was lost: %v", err)
		}
	})

	t.Run("index evidence", func(t *testing.T) {
		current := dependencies
		verified := verifiedDocumentCandidate(t, 1, 7, 0, "a", "first")
		current.Querier = fakeQuerier{result: contextindex.QueryBatchResult{
			Fusion: []contextindex.QueryFusionHit{{Point: verified.Point, Rank: 1, Score: 0.9}},
		}}
		_, err := Retrieve(t.Context(), input, current)
		assertEnhancementFailure(t, err, EnhancementStageIndex, ErrCodeIndexInconsistent)
	})

	t.Run("rerank", func(t *testing.T) {
		cause := errors.New("rerank provider unavailable")
		current := dependencies
		threshold, err := ParseFixedScore("0.500000")
		if err != nil {
			t.Fatal(err)
		}
		current.Reranker = &fakeRerankClient{err: cause}
		inputWithRerank := input
		inputWithRerank.RerankMinScore = &threshold
		current.Now = fiveMillisecondClock()
		result, retrieveErr := Retrieve(t.Context(), inputWithRerank, current)
		err = retrieveErr
		assertEnhancementFailure(t, err, EnhancementStageRerank, ErrCodeRerankFailed)
		if len(result.Candidates) != 0 || len(result.Excluded) != 0 || len(result.Cleanup) != 0 {
			t.Fatalf("failed retrieval retained partial evidence: %+v", result)
		}
		metrics := result.Metrics
		if metrics.Schema != ContextPlanMetricsSchemaV1 || metrics.QueryEmbeddingMS != 5 || metrics.RetrievalMS != 5 ||
			metrics.AuthorizationMS != 5 || metrics.RerankMS != 5 || metrics.QueryEmbeddingRequestCount != 1 ||
			metrics.RerankRequestCount != 1 || metrics.CandidateCount != 1 || metrics.QueryInputTokens == nil || *metrics.QueryInputTokens != 7 {
			t.Fatalf("partial metrics=%+v", metrics)
		}
	})
}

func TestRetrievalUnknownValidationErrorIsNotDegraded(t *testing.T) {
	input, dependencies := retrievalClassificationFixture(t)
	input.TopN = 0
	_, err := Retrieve(t.Context(), input, dependencies)
	if !errors.Is(err, ErrInvalidContextPlan) {
		t.Fatalf("validation error = %v", err)
	}
	if _, ok := AsEnhancementFailure(err); ok {
		t.Fatalf("validation error became degradable: %v", err)
	}
}

func retrievalClassificationFixture(t *testing.T) (RetrievalInput, RetrievalDependencies) {
	t.Helper()
	counter, err := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := contextindex.NewScopeFilter(1, 1, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified := verifiedDocumentCandidate(t, 1, 7, 0, "a", "first")
	return RetrievalInput{
			Collection: "admin_context_profile_1_g1", Filter: filter,
			Variants: []QueryVariant{{VariantID: "current", Text: "refund", QuerySHA256: hashText("query"), Sparse: contextindex.SparseVector{Indices: []uint32{1}, Values: []float32{1}}}},
			TopN:     20, Authority: CandidateAuthoritySnapshot{ProfileID: 1, IndexGeneration: 1, AgentID: 3, UserID: 4, ConversationID: 5, Platform: "admin"},
			MaxMergedTokens: 100, TokenCounter: counter,
		}, RetrievalDependencies{
			Embedding: fakeEmbeddingClient{result: infraai.EmbeddingResult{ModelID: "embed-v1", Vectors: [][]float32{{1, 0}}, Usage: infraai.EmbeddingUsage{PromptTokens: 7, TotalTokens: 7}}},
			Querier: fakeQuerier{result: contextindex.QueryBatchResult{
				Fusion:   []contextindex.QueryFusionHit{{Point: verified.Point, Rank: 1, Score: 0.9}},
				Branches: []contextindex.QueryBranchHit{{Point: verified.Point, VariantID: "current", Modality: contextindex.QueryModalityDense, Rank: 1, Score: 0.8}},
			}},
			Authority: fakeCandidateAuthority{result: CandidateVerification{Authorized: []VerifiedCandidate{verified}}},
		}
}

func fiveMillisecondClock() func() time.Time {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		current := now
		now = now.Add(5 * time.Millisecond)
		return current
	}
}

func assertEnhancementFailure(t *testing.T, err error, stage EnhancementStage, code ErrorCode) {
	t.Helper()
	failure, ok := AsEnhancementFailure(err)
	if !ok || failure.Stage != stage || failure.Code != code {
		t.Fatalf("failure=%#v ok=%v err=%v", failure, ok, err)
	}
}

type fakeRerankClient struct {
	result infraai.RerankResult
	err    error
}

type fakeEmbeddingClient struct {
	result infraai.EmbeddingResult
	err    error
}

func (client fakeEmbeddingClient) Embed(context.Context, []string) (infraai.EmbeddingResult, error) {
	return client.result, client.err
}

type fakeQuerier struct {
	result contextindex.QueryBatchResult
	err    error
}

func (client fakeQuerier) QueryBatch(context.Context, contextindex.QueryBatchInput) (contextindex.QueryBatchResult, error) {
	return client.result, client.err
}

type fakeCandidateAuthority struct {
	result CandidateVerification
	err    error
}

func (reader fakeCandidateAuthority) VerifyCandidates(context.Context, CandidateAuthoritySnapshot, []Candidate) (CandidateVerification, error) {
	return reader.result, reader.err
}

func (client *fakeRerankClient) Rerank(context.Context, string, []infraai.RerankDocument) (infraai.RerankResult, error) {
	return client.result, client.err
}

func verifiedDocumentCandidate(t *testing.T, sourceID, versionID uint64, ordinal uint32, contentKey, content string) VerifiedCandidate {
	t.Helper()
	ref := contextindex.PointRef{ID: uuid.MustParse("80000000-0000-8000-8000-" + leftPad12(sourceID)), ProfileID: 1, IndexGeneration: 1, SourceKind: contextindex.SourceKindDocumentChunk, SourceID: sourceID, SourceSHA256: hashText("facts-" + contentKey + leftPad12(sourceID))}
	fusion, _ := ParseFixedScore("0.900000")
	paragraph := ordinal + 1
	return VerifiedCandidate{
		Candidate:  Candidate{Point: ref, FusionScore: fusion, Branches: RetrievalBranchesV1{Schema: RetrievalBranchesSchemaV1, Branches: []RetrievalBranchV1{{VariantID: "current", Modality: "dense", Rank: sourceID, Score: fusion}}}},
		SourceType: "document_chunk", SourceSHA256: ref.SourceSHA256, Title: "Policy", DocumentID: 10, DocumentVersionID: versionID, ChunkIDs: []uint64{sourceID}, ChunkOrdinals: []uint32{ordinal},
		ChunkFactsSHA256: [][32]byte{ref.SourceSHA256}, ContentSHA256: hashText(contentKey), Content: content, TokenUpperBound: 5,
		Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}},
	}
}

func leftPad12(value uint64) string { return fmt.Sprintf("%012d", value) }
