package contextengine

import (
	"context"
	"crypto/sha256"
	"reflect"
	"regexp"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGormEvaluationFactsReaderLoadsCurrentAgentProfileModelAndSpaces(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	profileID := uint64(7)
	generation := uint64(3)

	mock.ExpectQuery(`(?s)SELECT .*agent\.id AS agent_id.*FROM ai_agents AS agent.*JOIN ai_providers AS provider.*JOIN ai_provider_models AS model.*WHERE agent\.id = \?.*LIMIT \?`).
		WithArgs(uint64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id", "agent_provider_id", "agent_model_id", "agent_system_prompt", "agent_context_profile_id",
			"agent_status", "agent_deleted", "provider_id", "provider_engine_type", "provider_base_url",
			"provider_api_protocol", "provider_status", "provider_deleted", "provider_model_id", "provider_model_kind",
			"provider_model_status", "official_model_id", "official_catalog_version", "mapping_status",
		}).AddRow(uint64(9), uint64(2), "chat-v1", "system", profileID, enum.CommonYes, enum.CommonNo,
			uint64(2), "openai", "https://api.example.com/v1", "responses", enum.CommonYes, enum.CommonNo,
			uint64(5), string(aiprovider.ModelKindChat), enum.CommonYes, "chat-v1", "catalog-v1", officialmodel.MappingStatusMapped))
	mock.ExpectQuery(`(?s)SELECT .*binding\.id AS binding_id.*FROM ai_agent_tools AS binding.*WHERE binding\.agent_id = \?.*ORDER BY binding\.id ASC`).
		WithArgs(uint64(9), enum.CommonYes, enum.CommonYes, enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"binding_id", "tool_id", "code", "description", "parameters_json"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_context_profiles` WHERE id = ? LIMIT ?")).
		WithArgs(profileID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "embedding_max_input_tokens", "embedding_token_counter_id", "dense_min_score", "status", "active_index_generation", "index_state",
		}).AddRow(profileID, int64(4096), infraai.TokenCounterUTF8BytesV1, "0.100000", ProfileEnabled, generation, ProfileIndexReady))
	mock.ExpectQuery(`(?s)SELECT .*binding\.id.*binding\.space_id.*space\.profile_id FROM ai_context_bindings AS binding.*WHERE .*binding\.agent_id = \?.*space\.profile_id = \?.*ORDER BY binding\.id ASC`).
		WithArgs(uint64(9), SpaceEnabled, SpaceEnabled, profileID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "space_id", "profile_id"}).
			AddRow(uint64(1), uint64(11), profileID).
			AddRow(uint64(2), uint64(12), profileID))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM ai_context_documents AS document.*JOIN ai_context_document_versions AS version.*document\.space_id IN \(\?,\?\)`).
		WithArgs(DocumentVersionReady, profileID, DocumentEnabled, uint64(11), uint64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	requestedModelID := ""
	resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		requestedModelID = modelID
		return officialmodel.ResolvedModel{Model: officialmodel.Model{
			ModelID: "chat-v1", CatalogVersion: "catalog-v1", LifecycleStatus: officialmodel.LifecycleActive,
			ContextWindowTokens: 128_000, MaxOutputTokens: 8_192, TokenCounterID: infraai.TokenCounterUTF8BytesV1,
		}}, nil
	})
	reader := NewEvaluationFactsReader(&database.Client{Gorm: planRepository.db}, resolver)
	facts, err := reader.LoadEvaluationFacts(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if requestedModelID != "chat-v1" || facts.Profile.ID != profileID || !facts.HasSources ||
		!reflect.DeepEqual(facts.SpaceIDs, []uint64{11, 12}) || facts.Budget.KnownInputBudget != 113_664 {
		t.Fatalf("requested model=%q facts=%+v", requestedModelID, facts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetrievalEvaluationPipelineUsesAgentSpacesWithoutConversationScope(t *testing.T) {
	generation := uint64(3)
	profile := ContextProfile{
		ID: 7, Status: ProfileEnabled, IndexState: ProfileIndexReady, ActiveIndexGeneration: &generation,
		EmbeddingMaxInputTokens: 4096, EmbeddingTokenCounterID: infraai.TokenCounterUTF8BytesV1,
		DenseMinScore: "0.100000",
	}
	budget := Budget{
		ContextWindowTokens: 128_000, EffectiveOutputTokens: 8_192,
		ProviderProtocolUpperBound: 2_048, PolicySafetyMargin: 4_096,
		KnownInputBudget: 113_664, Proof: BudgetConservative,
	}
	point := contextindex.PointRef{
		ID:        uuid.MustParse("80000000-0000-8000-8000-000000000041"),
		ProfileID: 7, IndexGeneration: generation, SourceKind: contextindex.SourceKindDocumentChunk,
		SourceID: 41, SourceSHA256: sha256.Sum256([]byte("document facts")),
	}
	embedding := &evaluationEmbeddingClient{}
	querier := &evaluationQuerier{point: point}
	authority := &evaluationAuthority{}
	pipeline := NewRetrievalEvaluationPipeline(EvaluationPipelineDependencies{
		Facts: evaluationFactsReader{facts: EvaluationFacts{
			Profile: &profile, SpaceIDs: []uint64{11, 12}, HasSources: true, Budget: budget,
		}},
		Embeddings: evaluationEmbeddingResolver{client: embedding},
		Querier:    querier, Authority: authority,
		Platform: "admin", CollectionPrefix: "admin_context",
	})

	result, err := pipeline.Evaluate(context.Background(), 9, "  退款需要多久？  ")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != RetrievalHit || result.Budget != budget || len(result.Groups) != 1 {
		t.Fatalf("result=%+v", result)
	}
	if !reflect.DeepEqual(embedding.texts, []string{"退款需要多久?"}) {
		t.Fatalf("embedding texts=%q", embedding.texts)
	}
	filter := querier.input.Filter
	if filter.ProfileID != profile.ID || filter.IndexGeneration != generation || filter.Platform != "admin" ||
		!reflect.DeepEqual(filter.SpaceIDs, []uint64{11, 12}) || filter.Conversation != nil {
		t.Fatalf("scope filter=%+v", filter)
	}
	if authority.snapshot.AgentID != 9 || authority.snapshot.ProfileID != profile.ID ||
		authority.snapshot.IndexGeneration != generation || authority.snapshot.UserID != 0 || authority.snapshot.ConversationID != 0 {
		t.Fatalf("authority snapshot=%+v", authority.snapshot)
	}
}

func TestEvaluationFactsRequireActiveGenerationWhenSourcesExist(t *testing.T) {
	profile := ContextProfile{ID: 7}
	facts := EvaluationFacts{
		Profile: &profile, SpaceIDs: []uint64{11}, HasSources: true,
		Budget: Budget{
			ContextWindowTokens: 16_384, EffectiveOutputTokens: 4_096,
			ProviderProtocolUpperBound: 2_048, PolicySafetyMargin: 4_096,
			KnownInputBudget: 6_144, Proof: BudgetConservative,
		},
	}
	if err := facts.Validate(); err == nil {
		t.Fatal("sources require an active index generation")
	}
}

type evaluationFactsReader struct {
	facts EvaluationFacts
	err   error
}

func (reader evaluationFactsReader) LoadEvaluationFacts(context.Context, uint64) (EvaluationFacts, error) {
	return reader.facts, reader.err
}

type evaluationEmbeddingResolver struct{ client infraai.EmbeddingClient }

func (resolver evaluationEmbeddingResolver) ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error) {
	return resolver.client, nil
}

type evaluationEmbeddingClient struct{ texts []string }

func (client *evaluationEmbeddingClient) Embed(_ context.Context, texts []string) (infraai.EmbeddingResult, error) {
	client.texts = append([]string(nil), texts...)
	return infraai.EmbeddingResult{ModelID: "embed-v1", Vectors: [][]float32{{1, 0}}}, nil
}

type evaluationQuerier struct {
	input contextindex.QueryBatchInput
	point contextindex.PointRef
}

func (querier *evaluationQuerier) QueryBatch(_ context.Context, input contextindex.QueryBatchInput) (contextindex.QueryBatchResult, error) {
	querier.input = input
	return contextindex.QueryBatchResult{
		Fusion: []contextindex.QueryFusionHit{{Point: querier.point, Rank: 1, Score: 0.9}},
		Branches: []contextindex.QueryBranchHit{{
			Point: querier.point, VariantID: "current", Modality: contextindex.QueryModalityDense, Rank: 1, Score: 0.8,
		}},
	}, nil
}

type evaluationAuthority struct{ snapshot CandidateAuthoritySnapshot }

func (authority *evaluationAuthority) VerifyCandidates(_ context.Context, snapshot CandidateAuthoritySnapshot, candidates []Candidate) (CandidateVerification, error) {
	authority.snapshot = snapshot
	content := "退款通常在三个工作日内到账。"
	contentHash := sha256.Sum256([]byte(content))
	paragraph := uint32(1)
	return CandidateVerification{Authorized: []VerifiedCandidate{{
		Candidate: candidates[0], SourceType: "document_chunk", SourceSHA256: candidates[0].Point.SourceSHA256,
		Title: "退款规则", DocumentID: 21, DocumentVersionID: 31,
		ChunkIDs: []uint64{41}, ChunkOrdinals: []uint32{0}, ChunkFactsSHA256: [][sha256.Size]byte{candidates[0].Point.SourceSHA256},
		ContentSHA256: contentHash, Content: content, TokenUpperBound: 64,
		Locators: []ContextLocatorV1{{Schema: ContextLocatorSchemaV1, Kind: "paragraph", Paragraph: &paragraph}},
	}}}, nil
}
