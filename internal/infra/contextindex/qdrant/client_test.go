package qdrant

import (
	"context"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"testing"

	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
	qdrantapi "github.com/qdrant/go-client/qdrant"
)

type fakeAPI struct {
	created       *qdrantapi.CreateCollection
	indexedFields []string
	upserted      *qdrantapi.UpsertPoints
	queried       *qdrantapi.QueryBatchPoints
	queryResults  []*qdrantapi.BatchResult
}

func (api *fakeAPI) CreateCollection(_ context.Context, request *qdrantapi.CreateCollection) error {
	api.created = request
	return nil
}

func (api *fakeAPI) DeleteCollection(context.Context, string) error { return nil }

func (api *fakeAPI) CreateFieldIndex(_ context.Context, request *qdrantapi.CreateFieldIndexCollection) (*qdrantapi.UpdateResult, error) {
	api.indexedFields = append(api.indexedFields, request.FieldName)
	return &qdrantapi.UpdateResult{}, nil
}

func (api *fakeAPI) Upsert(_ context.Context, request *qdrantapi.UpsertPoints) (*qdrantapi.UpdateResult, error) {
	api.upserted = request
	return &qdrantapi.UpdateResult{}, nil
}

func (api *fakeAPI) QueryBatch(_ context.Context, request *qdrantapi.QueryBatchPoints) ([]*qdrantapi.BatchResult, error) {
	api.queried = request
	return api.queryResults, nil
}

func (api *fakeAPI) Close() error { return nil }

func TestCreateCollectionUsesNamedDenseSparseIDFAndClosedIndexes(t *testing.T) {
	api := &fakeAPI{}
	client := newClient(api)
	spec, err := contextindex.NewCollectionSpec("admin_context_profile_7_g3", 4, contextindex.DistanceCosine)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CreateCollection(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	dense := api.created.GetVectorsConfig().GetParamsMap().GetMap()[denseVectorName]
	if dense == nil || dense.GetSize() != 4 || dense.GetDistance() != qdrantapi.Distance_Cosine {
		t.Fatalf("dense=%v", dense)
	}
	sparse := api.created.GetSparseVectorsConfig().GetMap()[sparseVectorName]
	if sparse == nil || sparse.GetModifier() != qdrantapi.Modifier_Idf {
		t.Fatalf("sparse=%v", sparse)
	}
	wantIndexes := []string{
		"profile_id", "index_generation", "platform", "scope_kind",
		"source_kind", "space_id", "conversation_id", "user_id",
	}
	if !reflect.DeepEqual(api.indexedFields, wantIndexes) {
		t.Fatalf("indexed fields=%v", api.indexedFields)
	}
}

func TestUpsertSerializesClosedPayloadAndOmitsEmptySparse(t *testing.T) {
	api := &fakeAPI{}
	client := newClient(api)
	point := testDocumentPoint(t, contextindex.SparseVector{})
	if err := client.Upsert(context.Background(), "admin_context_profile_7_g3", []contextindex.IndexedPoint{point}); err != nil {
		t.Fatal(err)
	}

	if len(api.upserted.GetPoints()) != 1 {
		t.Fatalf("points=%d", len(api.upserted.GetPoints()))
	}
	encoded := api.upserted.GetPoints()[0]
	vectors := encoded.GetVectors().GetVectors().GetVectors()
	if _, ok := vectors[denseVectorName]; !ok {
		t.Fatal("dense vector missing")
	}
	if _, ok := vectors[sparseVectorName]; ok {
		t.Fatal("empty sparse vector must be omitted")
	}
	wantPayloadKeys := []string{
		"chunk_id", "document_id", "document_version_id", "index_generation",
		"platform", "profile_id", "scope_kind", "source_id", "source_kind",
		"source_sha256", "space_id",
	}
	if got := sortedKeys(encoded.GetPayload()); !reflect.DeepEqual(got, wantPayloadKeys) {
		t.Fatalf("payload keys=%v", got)
	}
}

func TestQueryHybridUsesOneBatchWithIndependentBranchesAndOfficialRRF(t *testing.T) {
	point := testDocumentPoint(t, contextindex.SparseVector{Indices: []uint32{9}, Values: []float32{1}})
	api := &fakeAPI{queryResults: []*qdrantapi.BatchResult{
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.9)}},
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.8)}},
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(point, 0.7)}},
	}}
	client := newClient(api)
	filter, err := contextindex.NewScopeFilter(7, 3, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := contextindex.NewHybridQuery("admin_context_profile_7_g3", filter, []contextindex.QueryVariant{{
		Dense:  []float32{1, 0, 0, 0},
		Sparse: contextindex.SparseVector{Indices: []uint32{9}, Values: []float32{1}},
	}}, 20)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryHybrid(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if len(api.queried.GetQueryPoints()) != 3 {
		t.Fatalf("batch queries=%d", len(api.queried.GetQueryPoints()))
	}
	fused := api.queried.GetQueryPoints()[2]
	if fused.GetQuery().GetRrf() == nil || len(fused.GetPrefetch()) != 2 {
		t.Fatalf("fused=%v", fused)
	}
	wantPayload := []string{
		"platform", "profile_id", "index_generation", "scope_kind", "source_kind", "source_id",
		"space_id", "conversation_id", "user_id", "document_id", "document_version_id", "chunk_id", "source_sha256",
	}
	for i, query := range api.queried.GetQueryPoints() {
		if got := query.GetWithPayload().GetInclude().GetFields(); !reflect.DeepEqual(got, wantPayload) {
			t.Fatalf("query %d payload fields=%v", i, got)
		}
	}
	if len(result.Branches) != 2 || len(result.Fused) != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestQueryHybridRejectsMalformedOptionalPayload(t *testing.T) {
	point := testDocumentPoint(t, contextindex.SparseVector{})
	encoded := scoredPoint(point, 0.9)
	encoded.Payload["space_id"] = qdrantapi.NewValueString("11")
	api := &fakeAPI{queryResults: []*qdrantapi.BatchResult{
		{Result: []*qdrantapi.ScoredPoint{encoded}},
		{Result: []*qdrantapi.ScoredPoint{encoded}},
	}}
	client := newClient(api)
	filter, err := contextindex.NewScopeFilter(7, 3, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := contextindex.NewHybridQuery("admin_context_profile_7_g3", filter, []contextindex.QueryVariant{{Dense: []float32{1}}}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.QueryHybrid(context.Background(), request); err == nil {
		t.Fatal("malformed optional payload accepted")
	} else if !strings.Contains(err.Error(), `point payload "space_id" must be a positive integer`) {
		t.Fatalf("error=%v", err)
	}
}

func TestQueryHybridRejectsFusedPointMissingFromIndependentBranches(t *testing.T) {
	branchPoint := testDocumentPoint(t, contextindex.SparseVector{})
	fusedPoint := branchPoint
	fusedPoint.Metadata.Ref.ID = mustUUID(t, "018f3f5e-7b4c-8123-8abc-0123456789ac")
	fusedPoint.Metadata.Ref.SourceID = 42
	fusedPoint.Metadata.Ref.SourceSHA256 = [32]byte{2}
	fusedPoint.Metadata.ChunkID = 42
	api := &fakeAPI{queryResults: []*qdrantapi.BatchResult{
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(branchPoint, 0.9)}},
		{Result: []*qdrantapi.ScoredPoint{scoredPoint(fusedPoint, 0.8)}},
	}}
	client := newClient(api)
	filter, err := contextindex.NewScopeFilter(7, 3, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := contextindex.NewHybridQuery("admin_context_profile_7_g3", filter, []contextindex.QueryVariant{{Dense: []float32{1}}}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.QueryHybrid(context.Background(), request); err == nil {
		t.Fatal("orphan fused point accepted")
	} else if !strings.Contains(err.Error(), "not present in any independent branch") {
		t.Fatalf("error=%v", err)
	}
}

func testDocumentPoint(t *testing.T, sparse contextindex.SparseVector) contextindex.IndexedPoint {
	t.Helper()
	ref := contextindex.PointRef{
		ID:              mustUUID(t, "018f3f5e-7b4c-8123-8abc-0123456789ab"),
		ProfileID:       7,
		IndexGeneration: 3,
		SourceKind:      contextindex.SourceKindDocumentChunk,
		SourceID:        41,
		SourceSHA256:    [32]byte{1},
	}
	point, err := contextindex.NewIndexedPoint(contextindex.PointMetadata{
		Ref:               ref,
		Platform:          "admin",
		ScopeKind:         contextindex.ScopeKindSpace,
		SpaceID:           11,
		DocumentID:        31,
		DocumentVersionID: 37,
		ChunkID:           41,
	}, []float32{1, 0, 0, 0}, sparse)
	if err != nil {
		t.Fatal(err)
	}
	return point
}

func scoredPoint(point contextindex.IndexedPoint, score float32) *qdrantapi.ScoredPoint {
	metadata := point.Metadata
	return &qdrantapi.ScoredPoint{
		Id:    qdrantapi.NewID(metadata.Ref.ID.String()),
		Score: score,
		Payload: qdrantapi.NewValueMap(map[string]any{
			"profile_id":          metadata.Ref.ProfileID,
			"index_generation":    metadata.Ref.IndexGeneration,
			"platform":            metadata.Platform,
			"scope_kind":          string(metadata.ScopeKind),
			"source_kind":         string(metadata.Ref.SourceKind),
			"source_id":           metadata.Ref.SourceID,
			"space_id":            metadata.SpaceID,
			"document_id":         metadata.DocumentID,
			"document_version_id": metadata.DocumentVersionID,
			"chunk_id":            metadata.ChunkID,
			"source_sha256":       hex.EncodeToString(metadata.Ref.SourceSHA256[:]),
		}),
	}
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
