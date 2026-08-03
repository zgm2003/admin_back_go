package contextengine

import (
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/module/ai/officialmodel"
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
	planner := NewPlanner(PlannerDependencies{Repository: NewPlanRepository(dependencies.Database),
		GuardFactory: NewAuthorizationGuardFactory(NewAuthoritySnapshotLoader(dependencies.Platform), nil)})
	return NewRuntimeService(materializer, planner, NewDispatchGuardFactory(dependencies.Database, dependencies.Platform, nil))
}
