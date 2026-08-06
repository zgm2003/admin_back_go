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
	if dependencies.Embeddings == nil || dependencies.Querier == nil || dependencies.Authority == nil ||
		strings.TrimSpace(dependencies.Platform) == "" || strings.TrimSpace(dependencies.Prefix) == "" {
		return nil
	}
	return &RetrievalEvidenceResolver{embeddings: dependencies.Embeddings, rerank: dependencies.Rerank, querier: dependencies.Querier,
		authority: dependencies.Authority, turns: dependencies.Turns, platform: strings.ToLower(strings.TrimSpace(dependencies.Platform)), prefix: dependencies.Prefix}
}

func (resolver *RetrievalEvidenceResolver) ResolveRuntimeEvidence(ctx context.Context, input RuntimeInput, facts RuntimeFacts) (RuntimeEvidence, error) {
	metrics := ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}
	if resolver == nil || facts.Retrieval == nil {
		return RuntimeEvidence{Metrics: metrics}, ErrPlanRepositoryNotConfigured
	}
	retrievalFacts := facts.Retrieval
	if !retrievalFacts.HasSources {
		return RuntimeEvidence{Outcome: RetrievalSkipped, Metrics: metrics}, nil
	}
	profile := retrievalFacts.Profile
	if profile.IndexState == ProfileIndexFailed {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexFailed, nil)
	}
	if profile.Status != ProfileEnabled ||
		(profile.IndexState != ProfileIndexReady && profile.IndexState != ProfileIndexRebuilding) ||
		facts.Profile == nil || facts.Profile.IndexGeneration == nil || *facts.Profile.IndexGeneration == 0 {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, nil)
	}
	if resolver.turns == nil {
		return RuntimeEvidence{Metrics: metrics}, ErrPlanRepositoryNotConfigured
	}
	return resolver.resolve(ctx, retrievalRequest{
		Profile: profile, IndexGeneration: *facts.Profile.IndexGeneration, AgentID: input.AgentID,
		UserID: input.UserID, ConversationID: input.ConversationID, SpaceIDs: retrievalFacts.SpaceIDs,
		CurrentText: retrievalFacts.CurrentText,
		LoadNewest: func(ctx context.Context) (*ConversationTurn, error) {
			return resolver.turns.NewestComplete(ctx, input.ConversationID, input.UserID, &input.CurrentMessageID)
		},
	})
}

type retrievalRequest struct {
	Profile         ContextProfile
	IndexGeneration uint64
	AgentID         uint64
	UserID          uint64
	ConversationID  uint64
	SpaceIDs        []uint64
	CurrentText     string
	Newest          *ConversationTurn
	LoadNewest      func(context.Context) (*ConversationTurn, error)
}

func (resolver *RetrievalEvidenceResolver) resolve(ctx context.Context, request retrievalRequest) (RuntimeEvidence, error) {
	metrics := ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}
	if resolver == nil || resolver.embeddings == nil || resolver.querier == nil || resolver.authority == nil ||
		request.AgentID == 0 || request.IndexGeneration == 0 {
		return RuntimeEvidence{Metrics: metrics}, ErrPlanRepositoryNotConfigured
	}
	profile := request.Profile
	if profile.IndexState == ProfileIndexFailed {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexFailed, nil)
	}
	if profile.Status != ProfileEnabled ||
		(profile.IndexState != ProfileIndexReady && profile.IndexState != ProfileIndexRebuilding) {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, nil)
	}
	embedding, err := resolver.embeddings.ResolveEmbedding(ctx, profile)
	if err != nil {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, err)
	}
	counter, err := infraai.ResolveTokenCounter(profile.EmbeddingTokenCounterID)
	if err != nil {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, err)
	}
	newest := request.Newest
	if request.LoadNewest != nil {
		newest, err = request.LoadNewest(ctx)
		if err != nil {
			return RuntimeEvidence{Metrics: metrics}, err
		}
	}
	variants, err := BuildQueryVariants(request.CurrentText, newest, counter, profile.EmbeddingMaxInputTokens)
	if err != nil {
		return RuntimeEvidence{Metrics: metrics}, err
	}
	if len(variants) == 0 {
		return RuntimeEvidence{Outcome: RetrievalNoHit, Metrics: metrics}, nil
	}
	var conversation *contextindex.ConversationScope
	if request.UserID != 0 || request.ConversationID != 0 {
		conversation = &contextindex.ConversationScope{ConversationID: request.ConversationID, UserID: request.UserID}
	}
	filter, err := contextindex.NewScopeFilter(profile.ID, request.IndexGeneration, resolver.platform, request.SpaceIDs, conversation)
	if err != nil {
		return RuntimeEvidence{Metrics: metrics}, err
	}
	denseMin, err := ParseFixedScore(profile.DenseMinScore)
	if err != nil {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, err)
	}
	var rerankMin *FixedScore
	var reranker infraai.RerankClient
	if profile.RerankerMinScore != nil {
		rankScore, parseErr := ParseFixedScore(*profile.RerankerMinScore)
		if parseErr != nil || profile.RerankerProviderModelID == nil || resolver.rerank == nil {
			if parseErr == nil {
				parseErr = ErrInvalidContextPlan
			}
			return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageProfile, ErrCodeProfileUnavailable, parseErr)
		}
		rerankMin = &rankScore
		reranker, err = resolver.rerank.ResolveRerank(ctx, profile)
		if err != nil {
			return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: metrics}, NewEnhancementFailure(EnhancementStageRerank, ErrCodeRerankFailed, err)
		}
	}
	result, err := Retrieve(ctx, RetrievalInput{
		Collection: physicalCollectionName(resolver.prefix, profile.ID, request.IndexGeneration),
		Filter:     filter, Variants: variants, DenseMinScore: &denseMin, TopN: 50,
		Authority: CandidateAuthoritySnapshot{ProfileID: profile.ID, IndexGeneration: request.IndexGeneration,
			AgentID: request.AgentID, UserID: request.UserID, ConversationID: request.ConversationID, Platform: resolver.platform},
		MaxMergedTokens: profile.EmbeddingMaxInputTokens, TokenCounter: counter, RerankMinScore: rerankMin,
	}, RetrievalDependencies{Embedding: embedding, Querier: resolver.querier, Authority: resolver.authority, Reranker: reranker})
	if err != nil {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: result.Metrics}, err
	}
	if result.Outcome != RetrievalHit {
		return RuntimeEvidence{Outcome: result.Outcome, Metrics: result.Metrics}, nil
	}
	groups, err := retrievalPackGroups(result.Candidates)
	if err != nil {
		return RuntimeEvidence{Outcome: RetrievalFailed, Metrics: result.Metrics}, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexInconsistent, err)
	}
	return RuntimeEvidence{Outcome: RetrievalHit, Groups: groups, Metrics: result.Metrics}, nil
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
