//go:build integration

package qdrant_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
	qdrantapi "github.com/qdrant/go-client/qdrant"
)

const (
	denseVectorName  = "dense"
	sparseVectorName = "sparse"
)

func TestServerSupportsContextQueryContract(t *testing.T) {
	client := mustIntegrationClient(t)
	collection := uniqueCollection(t)
	createDenseSparseIDFCollection(t, client, collection, 4)
	upsertContractPoints(t, client, collection)

	filter, err := contextindex.NewScopeFilter(7, 1, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := queryBatchDenseSparseAndRRF(t, client, collection, filter)
	assertIndependentBranchRanks(t, results)
	assertOfficialRRFOrder(t, results)
	assertFilterExcludesOtherSpace(t, results)
}

func mustIntegrationClient(t *testing.T) *qdrantapi.Client {
	t.Helper()
	address := os.Getenv("QDRANT_INTEGRATION_ADDR")
	if address == "" {
		t.Fatal("QDRANT_INTEGRATION_ADDR is required")
	}
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("parse QDRANT_INTEGRATION_ADDR: %v", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port <= 0 || port > 65535 {
		t.Fatalf("invalid QDRANT_INTEGRATION_ADDR port %q", rawPort)
	}

	client, err := qdrantapi.NewClient(&qdrantapi.Config{
		Host:                host,
		Port:                port,
		PoolSize:            1,
		VersionCheckTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect Qdrant candidate: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Qdrant client: %v", err)
		}
	})
	return client
}

func uniqueCollection(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("admin_context_contract_%d_%s", time.Now().UnixNano(), uuid.NewString()[:8])
}

func createDenseSparseIDFCollection(t *testing.T, client *qdrantapi.Client, collection string, dimensions uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := client.CreateCollection(ctx, &qdrantapi.CreateCollection{
		CollectionName: collection,
		VectorsConfig: qdrantapi.NewVectorsConfigMap(map[string]*qdrantapi.VectorParams{
			denseVectorName: {
				Size:     dimensions,
				Distance: qdrantapi.Distance_Cosine,
			},
		}),
		SparseVectorsConfig: qdrantapi.NewSparseVectorsConfig(map[string]*qdrantapi.SparseVectorParams{
			sparseVectorName: {Modifier: qdrantapi.Modifier_Idf.Enum()},
		}),
	})
	if err != nil {
		t.Fatalf("create contract collection: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := client.DeleteCollection(cleanupCtx, collection); err != nil {
			t.Errorf("delete contract collection %q: %v", collection, err)
		}
	})

	info, err := client.GetCollectionInfo(ctx, collection)
	if err != nil {
		t.Fatalf("read contract collection schema: %v", err)
	}
	sparse := info.GetConfig().GetParams().GetSparseVectorsConfig().GetMap()[sparseVectorName]
	if sparse == nil || sparse.GetModifier() != qdrantapi.Modifier_Idf {
		t.Fatalf("sparse modifier=%v, want IDF", sparse)
	}
}

func upsertContractPoints(t *testing.T, client *qdrantapi.Client, collection string) {
	t.Helper()
	wait := true
	points := []*qdrantapi.PointStruct{
		contractPoint("018f3f5e-7b4c-8123-8abc-012345678901", 11, []float32{1, 0, 0, 0}, []uint32{1}, []float32{1}),
		contractPoint("018f3f5e-7b4c-8123-8abc-012345678902", 11, []float32{0.8, 0.2, 0, 0}, []uint32{2}, []float32{1}),
		contractPoint("018f3f5e-7b4c-8123-8abc-012345678903", 12, []float32{1, 0, 0, 0}, []uint32{2}, []float32{10}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := client.Upsert(ctx, &qdrantapi.UpsertPoints{
		CollectionName: collection,
		Wait:           &wait,
		Points:         points,
	}); err != nil {
		t.Fatalf("upsert contract points: %v", err)
	}
}

func contractPoint(id string, spaceID uint64, dense []float32, sparseIndices []uint32, sparseValues []float32) *qdrantapi.PointStruct {
	return &qdrantapi.PointStruct{
		Id: qdrantapi.NewID(id),
		Vectors: qdrantapi.NewVectorsMap(map[string]*qdrantapi.Vector{
			denseVectorName:  qdrantapi.NewVectorDense(dense),
			sparseVectorName: qdrantapi.NewVectorSparse(sparseIndices, sparseValues),
		}),
		Payload: qdrantapi.NewValueMap(map[string]any{
			"profile_id":       uint64(7),
			"index_generation": uint64(1),
			"platform":         "admin",
			"scope_kind":       "space",
			"space_id":         spaceID,
		}),
	}
}

func queryBatchDenseSparseAndRRF(
	t *testing.T,
	client *qdrantapi.Client,
	collection string,
	scope contextindex.ScopeFilter,
) []*qdrantapi.BatchResult {
	t.Helper()
	filter := scopeFilter(scope)
	limit := uint64(3)
	dense := &qdrantapi.PrefetchQuery{
		Query:  qdrantapi.NewQueryDense([]float32{1, 0, 0, 0}),
		Using:  stringPointer(denseVectorName),
		Filter: filter,
		Limit:  &limit,
	}
	sparse := &qdrantapi.PrefetchQuery{
		Query:  qdrantapi.NewQuerySparse([]uint32{2}, []float32{1}),
		Using:  stringPointer(sparseVectorName),
		Filter: filter,
		Limit:  &limit,
	}
	k := uint32(60)
	request := &qdrantapi.QueryBatchPoints{
		CollectionName: collection,
		QueryPoints: []*qdrantapi.QueryPoints{
			{CollectionName: collection, Query: dense.Query, Using: dense.Using, Filter: filter, Limit: &limit},
			{CollectionName: collection, Query: sparse.Query, Using: sparse.Using, Filter: filter, Limit: &limit},
			{
				CollectionName: collection,
				Prefetch:       []*qdrantapi.PrefetchQuery{dense, sparse},
				Query:          qdrantapi.NewQueryRRF(&qdrantapi.Rrf{K: &k}),
				Filter:         filter,
				Limit:          &limit,
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	results, err := client.QueryBatch(ctx, request)
	if err != nil {
		t.Fatalf("query contract batch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("batch results=%d, want 3", len(results))
	}
	return results
}

func scopeFilter(scope contextindex.ScopeFilter) *qdrantapi.Filter {
	spaceIDs := make([]int64, len(scope.SpaceIDs))
	for i, id := range scope.SpaceIDs {
		spaceIDs[i] = int64(id)
	}
	branches := make([]*qdrantapi.Condition, 0, 2)
	if len(spaceIDs) > 0 {
		branches = append(branches, qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchKeyword("scope_kind", "space"),
			qdrantapi.NewMatchKeyword("platform", scope.Platform),
			qdrantapi.NewMatchInts("space_id", spaceIDs...),
		}}))
	}
	if scope.Conversation != nil {
		branches = append(branches, qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchKeyword("scope_kind", "conversation"),
			qdrantapi.NewMatchKeyword("platform", scope.Platform),
			qdrantapi.NewMatchInt("conversation_id", int64(scope.Conversation.ConversationID)),
			qdrantapi.NewMatchInt("user_id", int64(scope.Conversation.UserID)),
		}}))
	}
	return &qdrantapi.Filter{
		Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchInt("profile_id", int64(scope.ProfileID)),
			qdrantapi.NewMatchInt("index_generation", int64(scope.IndexGeneration)),
			qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Should: branches}),
		},
	}
}

func assertIndependentBranchRanks(t *testing.T, results []*qdrantapi.BatchResult) {
	t.Helper()
	denseIDs := resultIDs(results[0])
	sparseIDs := resultIDs(results[1])
	if len(denseIDs) != 2 || denseIDs[0] != "018f3f5e-7b4c-8123-8abc-012345678901" {
		t.Fatalf("dense ids=%v", denseIDs)
	}
	if len(sparseIDs) != 1 || sparseIDs[0] != "018f3f5e-7b4c-8123-8abc-012345678902" {
		t.Fatalf("sparse ids=%v", sparseIDs)
	}
}

func assertOfficialRRFOrder(t *testing.T, results []*qdrantapi.BatchResult) {
	t.Helper()
	rrfIDs := resultIDs(results[2])
	if len(rrfIDs) != 2 || rrfIDs[0] != "018f3f5e-7b4c-8123-8abc-012345678902" || rrfIDs[1] != "018f3f5e-7b4c-8123-8abc-012345678901" {
		t.Fatalf("official RRF ids=%v", rrfIDs)
	}
}

func assertFilterExcludesOtherSpace(t *testing.T, results []*qdrantapi.BatchResult) {
	t.Helper()
	const excluded = "018f3f5e-7b4c-8123-8abc-012345678903"
	for resultIndex, result := range results {
		for _, id := range resultIDs(result) {
			if id == excluded {
				t.Fatalf("result %d included unauthorized space point", resultIndex)
			}
		}
	}
}

func resultIDs(result *qdrantapi.BatchResult) []string {
	ids := make([]string, len(result.GetResult()))
	for i, point := range result.GetResult() {
		ids[i] = point.GetId().GetUuid()
	}
	return ids
}

func stringPointer(value string) *string {
	return &value
}
