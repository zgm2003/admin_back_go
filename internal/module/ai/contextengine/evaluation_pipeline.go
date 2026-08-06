package contextengine

import (
	"context"
	"slices"
	"strings"

	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
)

type EvaluationFacts struct {
	Profile    *ContextProfile
	SpaceIDs   []uint64
	HasSources bool
	Budget     Budget
}

func (facts EvaluationFacts) Validate() error {
	if err := facts.Budget.Validate(); err != nil {
		return err
	}
	if facts.Profile == nil {
		if len(facts.SpaceIDs) != 0 || facts.HasSources {
			return ErrInvalidContextPlan
		}
		return nil
	}
	if facts.Profile.ID == 0 {
		return ErrInvalidContextPlan
	}
	if facts.HasSources && (len(facts.SpaceIDs) == 0 || facts.Profile.ActiveIndexGeneration == nil || *facts.Profile.ActiveIndexGeneration == 0) {
		return ErrInvalidContextPlan
	}
	return nil
}

type EvaluationFactsReader interface {
	LoadEvaluationFacts(context.Context, uint64) (EvaluationFacts, error)
}

type GormEvaluationFactsReader struct{ runtime *GormRuntimeFactsReader }

func NewEvaluationFactsReader(client *database.Client, resolver officialmodel.Resolver) *GormEvaluationFactsReader {
	runtime := NewRuntimeFactsReader(client, resolver)
	if runtime == nil {
		return nil
	}
	return &GormEvaluationFactsReader{runtime: runtime}
}

func (reader *GormEvaluationFactsReader) LoadEvaluationFacts(ctx context.Context, agentID uint64) (EvaluationFacts, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.db == nil || agentID == 0 {
		return EvaluationFacts{}, ErrPlanRepositoryNotConfigured
	}
	identity, err := reader.runtime.loadActiveIdentity(ctx, agentID)
	if err != nil {
		return EvaluationFacts{}, err
	}
	resolved, err := officialmodel.ResolveMappedRoute(ctx, reader.runtime.resolver, identity.AgentModelID,
		optionalString(identity.OfficialModelID), optionalString(identity.OfficialCatalog), identity.MappingStatus)
	if err != nil {
		return EvaluationFacts{}, err
	}
	tools, _, err := reader.runtime.loadTools(ctx, agentID)
	if err != nil {
		return EvaluationFacts{}, err
	}
	budget, err := runtimeBudget(resolved.Model, len(tools) > 0, nil)
	if err != nil {
		return EvaluationFacts{}, err
	}
	if identity.AgentContextProfileID == nil {
		return EvaluationFacts{Budget: budget}, nil
	}
	var profile ContextProfile
	if err := reader.runtime.db.WithContext(ctx).Where("id = ?", *identity.AgentContextProfileID).Take(&profile).Error; err != nil {
		return EvaluationFacts{}, err
	}
	bindings, err := reader.runtime.loadBindings(ctx, agentID, identity.AgentContextProfileID)
	if err != nil {
		return EvaluationFacts{}, err
	}
	spaceIDs := bindingSpaceIDs(bindings)
	hasSources, err := reader.hasSpaceSources(ctx, profile.ID, spaceIDs)
	if err != nil {
		return EvaluationFacts{}, err
	}
	facts := EvaluationFacts{Profile: &profile, SpaceIDs: spaceIDs, HasSources: hasSources, Budget: budget}
	if err := facts.Validate(); err != nil {
		return EvaluationFacts{}, err
	}
	return facts, nil
}

func (reader *GormEvaluationFactsReader) hasSpaceSources(ctx context.Context, profileID uint64, spaceIDs []uint64) (bool, error) {
	if len(spaceIDs) == 0 {
		return false, nil
	}
	var count int64
	err := reader.runtime.db.WithContext(ctx).Table("ai_context_documents AS document").
		Joins("JOIN ai_context_document_versions AS version ON version.id = document.active_version_id AND version.state = ? AND version.profile_id = ?", DocumentVersionReady, profileID).
		Where("document.status = ? AND document.deleted_at IS NULL AND document.space_id IN ?", DocumentEnabled, spaceIDs).
		Count(&count).Error
	return count > 0, err
}

type EvaluationPipelineDependencies struct {
	Facts            EvaluationFactsReader
	Embeddings       RuntimeEmbeddingResolver
	Rerank           RuntimeRerankResolver
	Querier          contextindex.Querier
	Authority        CandidateAuthorityReader
	Platform         string
	CollectionPrefix string
}

type RetrievalEvaluationPipeline struct {
	facts     EvaluationFactsReader
	retrieval *RetrievalEvidenceResolver
}

func NewRetrievalEvaluationPipeline(dependencies EvaluationPipelineDependencies) *RetrievalEvaluationPipeline {
	retrieval := NewRetrievalEvidenceResolver(RetrievalEvidenceDependencies{
		Embeddings: dependencies.Embeddings, Rerank: dependencies.Rerank, Querier: dependencies.Querier,
		Authority: dependencies.Authority, Platform: dependencies.Platform, Prefix: dependencies.CollectionPrefix,
	})
	if dependencies.Facts == nil || retrieval == nil {
		return nil
	}
	return &RetrievalEvaluationPipeline{facts: dependencies.Facts, retrieval: retrieval}
}

func (pipeline *RetrievalEvaluationPipeline) Evaluate(ctx context.Context, agentID uint64, query string) (EvaluationPipelineResult, error) {
	if pipeline == nil || pipeline.facts == nil || pipeline.retrieval == nil || agentID == 0 || !validEvaluationQuery(query) {
		return EvaluationPipelineResult{}, ErrInvalidContextPlan
	}
	facts, err := pipeline.facts.LoadEvaluationFacts(ctx, agentID)
	if err != nil {
		return EvaluationPipelineResult{}, err
	}
	if err := facts.Validate(); err != nil {
		return EvaluationPipelineResult{}, err
	}
	metrics := ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}
	if facts.Profile == nil || !facts.HasSources {
		return EvaluationPipelineResult{Outcome: RetrievalSkipped, Budget: facts.Budget, Metrics: metrics}, nil
	}
	evidence, err := pipeline.retrieval.resolve(ctx, retrievalRequest{
		Profile: *facts.Profile, IndexGeneration: *facts.Profile.ActiveIndexGeneration, AgentID: agentID,
		SpaceIDs: slices.Clone(facts.SpaceIDs), CurrentText: strings.TrimSpace(query),
	})
	if err != nil {
		return EvaluationPipelineResult{}, err
	}
	return EvaluationPipelineResult{Outcome: evidence.Outcome, Budget: facts.Budget, Metrics: evidence.Metrics, Groups: evidence.Groups}, nil
}

var _ EvaluationFactsReader = (*GormEvaluationFactsReader)(nil)
var _ EvaluationPipeline = (*RetrievalEvaluationPipeline)(nil)
