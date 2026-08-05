package contextengine

import (
	"context"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
)

type RuntimeEmbeddingResolver interface {
	ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error)
}

type RuntimeRerankResolver interface {
	ResolveRerank(context.Context, ContextProfile) (infraai.RerankClient, error)
}

type RetrievalEvidenceDependencies struct {
	Embeddings RuntimeEmbeddingResolver
	Rerank     RuntimeRerankResolver
	Querier    contextindex.Querier
	Authority  CandidateAuthorityReader
	Turns      ConversationTurnReader
	Platform   string
	Prefix     string
}

type RetrievalEvidenceResolver struct {
	embeddings RuntimeEmbeddingResolver
	rerank     RuntimeRerankResolver
	querier    contextindex.Querier
	authority  CandidateAuthorityReader
	turns      ConversationTurnReader
	platform   string
	prefix     string
}

func NewRetrievalEvidenceResolver(dependencies RetrievalEvidenceDependencies) *RetrievalEvidenceResolver {
	if dependencies.Embeddings == nil || dependencies.Querier == nil || dependencies.Authority == nil || dependencies.Turns == nil ||
		strings.TrimSpace(dependencies.Platform) == "" || strings.TrimSpace(dependencies.Prefix) == "" {
		return nil
	}
	return &RetrievalEvidenceResolver{embeddings: dependencies.Embeddings, rerank: dependencies.Rerank, querier: dependencies.Querier,
		authority: dependencies.Authority, turns: dependencies.Turns, platform: strings.ToLower(strings.TrimSpace(dependencies.Platform)), prefix: dependencies.Prefix}
}

func (resolver *RetrievalEvidenceResolver) ResolveRuntimeEvidence(ctx context.Context, input RuntimeInput, facts RuntimeFacts) (RuntimeEvidence, error) {
	if resolver == nil || facts.Retrieval == nil {
		return RuntimeEvidence{}, ErrPlanRepositoryNotConfigured
	}
	retrievalFacts := facts.Retrieval
	if !retrievalFacts.HasSources {
		return RuntimeEvidence{Outcome: RetrievalSkipped}, nil
	}
	profile := retrievalFacts.Profile
	if profile.IndexState == ProfileIndexFailed {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexFailed, nil)
	}
	if profile.Status != ProfileEnabled ||
		(profile.IndexState != ProfileIndexReady && profile.IndexState != ProfileIndexRebuilding) ||
		facts.Profile == nil || facts.Profile.IndexGeneration == nil || *facts.Profile.IndexGeneration == 0 {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, nil)
	}
	embedding, err := resolver.embeddings.ResolveEmbedding(ctx, profile)
	if err != nil {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, err)
	}
	counter, err := infraai.ResolveTokenCounter(profile.EmbeddingTokenCounterID)
	if err != nil {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, err)
	}
	newest, err := resolver.turns.NewestComplete(ctx, input.ConversationID, input.UserID, &input.CurrentMessageID)
	if err != nil {
		return RuntimeEvidence{}, err
	}
	variants, err := BuildQueryVariants(retrievalFacts.CurrentText, newest, counter, profile.EmbeddingMaxInputTokens)
	if err != nil {
		return RuntimeEvidence{}, err
	}
	if len(variants) == 0 {
		return RuntimeEvidence{Outcome: RetrievalNoHit}, nil
	}
	filter, err := contextindex.NewScopeFilter(profile.ID, *facts.Profile.IndexGeneration, resolver.platform, retrievalFacts.SpaceIDs,
		&contextindex.ConversationScope{ConversationID: input.ConversationID, UserID: input.UserID})
	if err != nil {
		return RuntimeEvidence{}, err
	}
	denseMin, err := ParseFixedScore(profile.DenseMinScore)
	if err != nil {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, err)
	}
	var rerankMin *FixedScore
	var reranker infraai.RerankClient
	if profile.RerankerMinScore != nil {
		rankScore, parseErr := ParseFixedScore(*profile.RerankerMinScore)
		if parseErr != nil || profile.RerankerProviderModelID == nil || resolver.rerank == nil {
			if parseErr == nil {
				parseErr = ErrInvalidContextPlan
			}
			return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, parseErr)
		}
		rerankMin = &rankScore
		reranker, err = resolver.rerank.ResolveRerank(ctx, profile)
		if err != nil {
			return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageRerank, ErrCodeRerankFailed, err)
		}
	}
	result, err := Retrieve(ctx, RetrievalInput{
		Collection: physicalCollectionName(resolver.prefix, profile.ID, *facts.Profile.IndexGeneration),
		Filter:     filter, Variants: variants, DenseMinScore: &denseMin, TopN: 50,
		Authority: CandidateAuthoritySnapshot{ProfileID: profile.ID, IndexGeneration: *facts.Profile.IndexGeneration,
			AgentID: input.AgentID, UserID: input.UserID, ConversationID: input.ConversationID, Platform: resolver.platform},
		MaxMergedTokens: profile.EmbeddingMaxInputTokens, TokenCounter: counter, RerankMinScore: rerankMin,
	}, RetrievalDependencies{Embedding: embedding, Querier: resolver.querier, Authority: resolver.authority, Reranker: reranker})
	if err != nil {
		return RuntimeEvidence{}, err
	}
	if result.Outcome != RetrievalHit {
		return RuntimeEvidence{Outcome: result.Outcome}, nil
	}
	groups, err := retrievalPackGroups(result.Candidates)
	if err != nil {
		return RuntimeEvidence{}, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexInconsistent, err)
	}
	return RuntimeEvidence{Outcome: RetrievalHit, Groups: groups}, nil
}

func retrievalPackGroups(candidates []VerifiedCandidate) ([]PackGroup, error) {
	groups := make([]PackGroup, 0, len(candidates))
	for index, candidate := range candidates {
		if err := validateVerifiedCandidate(candidate); err != nil {
			return nil, err
		}
		priority := int32(5)
		block := ContextBlock{SourceType: candidate.SourceType, SourceRef: candidate.CandidateID(), SourceSHA256: candidate.SourceSHA256,
			AtomicGroupKey: candidate.CandidateID(), Required: false, Priority: priority, TokenUpperBound: candidate.TokenUpperBound,
			ContentSnapshot: &candidate.Content, Metadata: ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1, Retrieval: &candidate.Branches}}
		if candidate.SourceType == "document_chunk" {
			block.Kind = BlockDocumentEvidence
			block.Metadata.Document = &ContextDocumentEvidenceV1{Title: candidate.Title, DocumentID: candidate.DocumentID, DocumentVersionID: candidate.DocumentVersionID,
				ChunkIDs: append([]uint64(nil), candidate.ChunkIDs...), Locators: append([]ContextLocatorV1(nil), candidate.Locators...)}
		} else if candidate.ConversationTurn != nil {
			priority = 8
			block.Kind = BlockRecalledTurn
			block.Priority = priority
		}
		groups = append(groups, PackGroup{Required: false, Priority: priority, Relevance: clonePointer(&candidate.FusionScore), SourceOrder: int64(index),
			StableSourceID: candidate.CandidateID(), Blocks: []PackBlock{{Block: block, FusionScore: clonePointer(&candidate.FusionScore), RerankScore: clonePointer(candidate.RerankScore)}}})
	}
	return groups, nil
}

var _ RuntimeEvidenceResolver = (*RetrievalEvidenceResolver)(nil)
