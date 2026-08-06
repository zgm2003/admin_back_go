package contextengine

import (
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/telemetry"
)

type RuntimeDependencies struct {
	Database         *database.Client
	OfficialModels   officialmodel.Resolver
	EmbeddingFactory infraai.EmbeddingFactory
	RerankFactory    infraai.RerankFactory
	Secretbox        secretbox.Box
	Index            contextindex.Querier
	CollectionPrefix string
	Platform         string
	Telemetry        telemetry.Recorder
}

func BuildEvaluationPipeline(dependencies RuntimeDependencies) *RetrievalEvaluationPipeline {
	return NewRetrievalEvaluationPipeline(EvaluationPipelineDependencies{
		Facts:      NewEvaluationFactsReader(dependencies.Database, dependencies.OfficialModels),
		Embeddings: NewEmbeddingResolver(dependencies.Database, dependencies.EmbeddingFactory, dependencies.Secretbox),
		Rerank:     NewRerankResolver(dependencies.Database, dependencies.RerankFactory, dependencies.Secretbox),
		Querier:    dependencies.Index, Authority: NewCandidateRepository(dependencies.Database),
		Platform: dependencies.Platform, CollectionPrefix: dependencies.CollectionPrefix,
	})
}

func BuildRuntime(dependencies RuntimeDependencies) *RuntimeService {
	facts := NewRuntimeFactsReader(dependencies.Database, dependencies.OfficialModels)
	embeddings := NewEmbeddingResolver(dependencies.Database, dependencies.EmbeddingFactory, dependencies.Secretbox)
	rerank := NewRerankResolver(dependencies.Database, dependencies.RerankFactory, dependencies.Secretbox)
	turns := NewConversationRepository(dependencies.Database)
	evidence := NewRetrievalEvidenceResolver(RetrievalEvidenceDependencies{
		Embeddings: embeddings, Rerank: rerank, Querier: dependencies.Index, Authority: NewCandidateRepository(dependencies.Database),
		Turns: turns, Platform: dependencies.Platform, Prefix: dependencies.CollectionPrefix,
	})
	materializer := NewPlanMaterializer(facts, evidence, turns)
	materializer.attachments = NewConversationDocumentService(dependencies.Database, nil)
	materializer.WithMemoryReader(NewMemoryRepository(dependencies.Database))
	planner := NewPlanner(PlannerDependencies{Repository: NewPlanRepository(dependencies.Database),
		GuardFactory: NewAuthorizationGuardFactory(NewAuthoritySnapshotLoader(dependencies.Platform), nil)})
	return NewRuntimeService(materializer, planner, NewDispatchGuardFactory(dependencies.Database, dependencies.Platform, nil)).WithTelemetry(dependencies.Telemetry)
}
