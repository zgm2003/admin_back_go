package contextengine

import (
	"context"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestRuntimeEvidenceRejectsFailedProfileOnlyWhenSourcesExist(t *testing.T) {
	generation := uint64(3)
	profile := ContextProfile{
		ID: 7, Status: ProfileEnabled, IndexState: ProfileIndexFailed, ActiveIndexGeneration: &generation,
		EmbeddingTokenCounterID: "utf8_bytes_v1", EmbeddingMaxInputTokens: 4096, DenseMinScore: "0.100000",
	}
	embeddings := &countingRuntimeEmbeddingResolver{}
	resolver := newRuntimeEvidenceTestResolver(embeddings, nil)

	evidence, err := resolver.ResolveRuntimeEvidence(context.Background(), RuntimeInput{AgentID: 9, ConversationID: 11, UserID: 13}, RuntimeFacts{
		Profile:   &ProfileSnapshot{ID: profile.ID, IndexGeneration: &generation},
		Retrieval: &RuntimeRetrievalFacts{Profile: profile, HasSources: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Outcome != RetrievalFailed || evidence.Failure == nil || evidence.Failure.Code != ErrCodeIndexFailed {
		t.Fatalf("evidence=%+v", evidence)
	}
	if embeddings.calls != 0 {
		t.Fatalf("embedding resolver calls=%d, want 0", embeddings.calls)
	}

	evidence, err = resolver.ResolveRuntimeEvidence(context.Background(), RuntimeInput{}, RuntimeFacts{
		Profile:   &ProfileSnapshot{ID: profile.ID, IndexGeneration: &generation},
		Retrieval: &RuntimeRetrievalFacts{Profile: profile, HasSources: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Outcome != RetrievalSkipped || evidence.Failure != nil {
		t.Fatalf("evidence=%+v", evidence)
	}
	if embeddings.calls != 0 {
		t.Fatalf("embedding resolver calls=%d, want 0", embeddings.calls)
	}
}

func TestRuntimeEvidenceMapsRerankResolverFailure(t *testing.T) {
	generation := uint64(3)
	rerankerModelID := uint64(19)
	rerankerMinScore := "0.200000"
	profile := ContextProfile{
		ID: 7, Status: ProfileEnabled, IndexState: ProfileIndexReady, ActiveIndexGeneration: &generation,
		EmbeddingTokenCounterID: "utf8_bytes_v1", EmbeddingMaxInputTokens: 4096, DenseMinScore: "0.100000",
		RerankerProviderModelID: &rerankerModelID, RerankerMinScore: &rerankerMinScore,
	}
	embeddings := &countingRuntimeEmbeddingResolver{client: fakeEmbeddingClient{}}
	rerank := &failingRuntimeRerankResolver{err: errors.New("rerank provider unavailable")}
	resolver := newRuntimeEvidenceTestResolver(embeddings, rerank)

	evidence, err := resolver.ResolveRuntimeEvidence(context.Background(), RuntimeInput{AgentID: 9, ConversationID: 11, UserID: 13}, RuntimeFacts{
		Profile:   &ProfileSnapshot{ID: profile.ID, IndexGeneration: &generation},
		Retrieval: &RuntimeRetrievalFacts{Profile: profile, CurrentText: "query", HasSources: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Outcome != RetrievalFailed || evidence.Failure == nil || evidence.Failure.Code != ErrCodeRerankFailed {
		t.Fatalf("evidence=%+v", evidence)
	}
	if rerank.calls != 1 {
		t.Fatalf("rerank resolver calls=%d, want 1", rerank.calls)
	}
}

type countingRuntimeEmbeddingResolver struct {
	calls  int
	client infraai.EmbeddingClient
}

func (resolver *countingRuntimeEmbeddingResolver) ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error) {
	resolver.calls++
	if resolver.client == nil {
		return nil, errors.New("unexpected embedding resolution")
	}
	return resolver.client, nil
}

type failingRuntimeRerankResolver struct {
	calls int
	err   error
}

func (resolver *failingRuntimeRerankResolver) ResolveRerank(context.Context, ContextProfile) (infraai.RerankClient, error) {
	resolver.calls++
	return nil, resolver.err
}

func newRuntimeEvidenceTestResolver(embeddings RuntimeEmbeddingResolver, rerank RuntimeRerankResolver) *RetrievalEvidenceResolver {
	return NewRetrievalEvidenceResolver(RetrievalEvidenceDependencies{
		Embeddings: embeddings,
		Rerank:     rerank,
		Querier:    fakeQuerier{},
		Authority:  fakeCandidateAuthority{},
		Turns:      emptyConversationTurnReader{},
		Platform:   "admin",
		Prefix:     "admin_context",
	})
}
