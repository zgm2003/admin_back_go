package qdrant

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"

	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
	qdrantapi "github.com/qdrant/go-client/qdrant"
)

const (
	denseVectorName  = "dense"
	sparseVectorName = "sparse"
	rrfK             = uint32(60)
)

var payloadIndexFields = []struct {
	name      string
	fieldType qdrantapi.FieldType
}{
	{name: "profile_id", fieldType: qdrantapi.FieldType_FieldTypeInteger},
	{name: "index_generation", fieldType: qdrantapi.FieldType_FieldTypeInteger},
	{name: "platform", fieldType: qdrantapi.FieldType_FieldTypeKeyword},
	{name: "scope_kind", fieldType: qdrantapi.FieldType_FieldTypeKeyword},
	{name: "source_kind", fieldType: qdrantapi.FieldType_FieldTypeKeyword},
	{name: "space_id", fieldType: qdrantapi.FieldType_FieldTypeInteger},
	{name: "conversation_id", fieldType: qdrantapi.FieldType_FieldTypeInteger},
	{name: "user_id", fieldType: qdrantapi.FieldType_FieldTypeInteger},
}

var pointPayloadFields = []string{
	"platform", "profile_id", "index_generation", "scope_kind", "source_kind", "source_id",
	"space_id", "conversation_id", "user_id", "document_id", "document_version_id", "chunk_id", "source_sha256",
}

type Config struct {
	Address string
	APIKey  string
	UseTLS  bool
}

type qdrantAPI interface {
	CreateCollection(context.Context, *qdrantapi.CreateCollection) error
	DeleteCollection(context.Context, string) error
	CreateFieldIndex(context.Context, *qdrantapi.CreateFieldIndexCollection) (*qdrantapi.UpdateResult, error)
	Upsert(context.Context, *qdrantapi.UpsertPoints) (*qdrantapi.UpdateResult, error)
	Get(context.Context, *qdrantapi.GetPoints) ([]*qdrantapi.RetrievedPoint, error)
	QueryBatch(context.Context, *qdrantapi.QueryBatchPoints) ([]*qdrantapi.BatchResult, error)
	Close() error
}

type Client struct {
	api qdrantAPI
}

func New(config Config) (*Client, error) {
	host, rawPort, err := net.SplitHostPort(strings.TrimSpace(config.Address))
	if err != nil {
		return nil, fmt.Errorf("parse Qdrant address: %w", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid Qdrant port %q", rawPort)
	}
	official, err := qdrantapi.NewClient(&qdrantapi.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 config.APIKey,
		UseTLS:                 config.UseTLS,
		PoolSize:               1,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create Qdrant client: %w", err)
	}
	return newClient(official), nil
}

func newClient(api qdrantAPI) *Client {
	return &Client{api: api}
}

func (client *Client) Close() error {
	if client == nil || client.api == nil {
		return nil
	}
	return client.api.Close()
}

func (client *Client) CreateCollection(ctx context.Context, spec contextindex.CollectionSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	distance, err := denseDistance(spec.DenseDistance)
	if err != nil {
		return err
	}
	if err := client.api.CreateCollection(ctx, &qdrantapi.CreateCollection{
		CollectionName: spec.Name,
		VectorsConfig: qdrantapi.NewVectorsConfigMap(map[string]*qdrantapi.VectorParams{
			denseVectorName: {
				Size:     spec.DenseDimensions,
				Distance: distance,
			},
		}),
		SparseVectorsConfig: qdrantapi.NewSparseVectorsConfig(map[string]*qdrantapi.SparseVectorParams{
			sparseVectorName: {Modifier: qdrantapi.Modifier_Idf.Enum()},
		}),
	}); err != nil {
		return fmt.Errorf("create Qdrant collection %q: %w", spec.Name, err)
	}

	wait := true
	for _, field := range payloadIndexFields {
		if _, err := client.api.CreateFieldIndex(ctx, &qdrantapi.CreateFieldIndexCollection{
			CollectionName: spec.Name,
			FieldName:      field.name,
			FieldType:      field.fieldType.Enum(),
			Wait:           &wait,
		}); err != nil {
			return fmt.Errorf("create Qdrant payload index %q on %q: %w", field.name, spec.Name, err)
		}
	}
	return nil
}

func (client *Client) DeleteCollection(ctx context.Context, collection string) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if err := client.api.DeleteCollection(ctx, collection); err != nil {
		return fmt.Errorf("delete Qdrant collection %q: %w", collection, err)
	}
	return nil
}

func (client *Client) Upsert(ctx context.Context, collection string, points []contextindex.IndexedPoint) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if len(points) == 0 {
		return errors.New("Qdrant upsert requires at least one point")
	}
	encoded := make([]*qdrantapi.PointStruct, len(points))
	for i, point := range points {
		if err := point.Validate(); err != nil {
			return fmt.Errorf("point %d: %w", i, err)
		}
		encoded[i] = encodePoint(point)
	}
	wait := true
	if _, err := client.api.Upsert(ctx, &qdrantapi.UpsertPoints{
		CollectionName: collection,
		Points:         encoded,
		Wait:           &wait,
	}); err != nil {
		return fmt.Errorf("upsert Qdrant points in %q: %w", collection, err)
	}
	return nil
}

func (client *Client) VerifyPoints(ctx context.Context, collection string, expected []contextindex.PointRef, denseDimensions uint32) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if len(expected) == 0 || denseDimensions == 0 {
		return errors.New("Qdrant point verification requires identities and dense dimensions")
	}
	want := make(map[uuid.UUID]contextindex.PointRef, len(expected))
	ids := make([]*qdrantapi.PointId, len(expected))
	for i, ref := range expected {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("expected point %d: %w", i, err)
		}
		if _, duplicate := want[ref.ID]; duplicate {
			return fmt.Errorf("duplicate expected point ID %s", ref.ID)
		}
		want[ref.ID] = ref
		ids[i] = qdrantapi.NewID(ref.ID.String())
	}
	points, err := client.api.Get(ctx, &qdrantapi.GetPoints{
		CollectionName: collection,
		Ids:            ids,
		WithPayload:    pointPayloadSelector(),
		WithVectors:    qdrantapi.NewWithVectorsInclude(denseVectorName),
	})
	if err != nil {
		return fmt.Errorf("verify Qdrant points in %q: %w", collection, err)
	}
	if len(points) != len(want) {
		return fmt.Errorf("Qdrant returned %d points, want %d", len(points), len(want))
	}
	seen := make(map[uuid.UUID]struct{}, len(points))
	for i, point := range points {
		if point == nil || point.GetId() == nil {
			return fmt.Errorf("Qdrant point %d has no ID", i)
		}
		metadata, err := decodeMetadata(point.GetId().GetUuid(), point.GetPayload())
		if err != nil {
			return fmt.Errorf("decode Qdrant point %d: %w", i, err)
		}
		expectedRef, ok := want[metadata.Ref.ID]
		if !ok || expectedRef != metadata.Ref {
			return fmt.Errorf("Qdrant point %s identity or source hash differs", metadata.Ref.ID)
		}
		if _, duplicate := seen[metadata.Ref.ID]; duplicate {
			return fmt.Errorf("Qdrant returned duplicate point %s", metadata.Ref.ID)
		}
		seen[metadata.Ref.ID] = struct{}{}
		vectors := point.GetVectors().GetVectors().GetVectors()
		dense := vectors[denseVectorName].GetDense()
		if dense == nil || len(dense.GetData()) != int(denseDimensions) {
			return fmt.Errorf("Qdrant point %s dense dimension differs", metadata.Ref.ID)
		}
	}
	return nil
}

func (client *Client) QueryHybrid(ctx context.Context, request contextindex.HybridQuery) (contextindex.HybridResult, error) {
	if err := request.Validate(); err != nil {
		return contextindex.HybridResult{}, err
	}
	filter := encodeScopeFilter(request.Filter)
	queryPoints := make([]*qdrantapi.QueryPoints, 0, len(request.Variants)*2+1)
	prefetch := make([]*qdrantapi.PrefetchQuery, 0, len(request.Variants)*2)
	branches := make([]contextindex.BranchResult, 0, len(request.Variants)*2)
	for variantIndex, variant := range request.Variants {
		dense := &qdrantapi.PrefetchQuery{
			Query:  qdrantapi.NewQueryDense(variant.Dense),
			Using:  stringPointer(denseVectorName),
			Filter: filter,
			Limit:  &request.Limit,
		}
		prefetch = append(prefetch, dense)
		queryPoints = append(queryPoints, independentQuery(request.Collection, dense))
		branches = append(branches, contextindex.BranchResult{VariantIndex: variantIndex, Modality: contextindex.ModalityDense})

		if !variant.Sparse.Empty() {
			sparse := &qdrantapi.PrefetchQuery{
				Query:  qdrantapi.NewQuerySparse(variant.Sparse.Indices, variant.Sparse.Values),
				Using:  stringPointer(sparseVectorName),
				Filter: filter,
				Limit:  &request.Limit,
			}
			prefetch = append(prefetch, sparse)
			queryPoints = append(queryPoints, independentQuery(request.Collection, sparse))
			branches = append(branches, contextindex.BranchResult{VariantIndex: variantIndex, Modality: contextindex.ModalitySparse})
		}
	}
	k := rrfK
	queryPoints = append(queryPoints, &qdrantapi.QueryPoints{
		CollectionName: request.Collection,
		Prefetch:       prefetch,
		Query:          qdrantapi.NewQueryRRF(&qdrantapi.Rrf{K: &k}),
		Filter:         filter,
		Limit:          &request.Limit,
		WithPayload:    pointPayloadSelector(),
	})

	results, err := client.api.QueryBatch(ctx, &qdrantapi.QueryBatchPoints{
		CollectionName: request.Collection,
		QueryPoints:    queryPoints,
	})
	if err != nil {
		return contextindex.HybridResult{}, fmt.Errorf("query Qdrant collection %q: %w", request.Collection, err)
	}
	if len(results) != len(branches)+1 {
		return contextindex.HybridResult{}, fmt.Errorf("Qdrant batch returned %d results, want %d", len(results), len(branches)+1)
	}
	for i := range branches {
		branches[i].Points, err = decodeScoredPoints(results[i].GetResult())
		if err != nil {
			return contextindex.HybridResult{}, fmt.Errorf("decode Qdrant branch %d: %w", i, err)
		}
	}
	fused, err := decodeScoredPoints(results[len(results)-1].GetResult())
	if err != nil {
		return contextindex.HybridResult{}, fmt.Errorf("decode Qdrant RRF result: %w", err)
	}
	branchPointIDs := make(map[uuid.UUID]struct{})
	for _, branch := range branches {
		for _, point := range branch.Points {
			branchPointIDs[point.Metadata.Ref.ID] = struct{}{}
		}
	}
	for _, point := range fused {
		if _, ok := branchPointIDs[point.Metadata.Ref.ID]; !ok {
			return contextindex.HybridResult{}, fmt.Errorf(
				"Qdrant RRF point %s is not present in any independent branch",
				point.Metadata.Ref.ID,
			)
		}
	}
	return contextindex.HybridResult{Branches: branches, Fused: fused}, nil
}

func denseDistance(distance contextindex.Distance) (qdrantapi.Distance, error) {
	switch distance {
	case contextindex.DistanceCosine:
		return qdrantapi.Distance_Cosine, nil
	case contextindex.DistanceDot:
		return qdrantapi.Distance_Dot, nil
	case contextindex.DistanceEuclid:
		return qdrantapi.Distance_Euclid, nil
	default:
		return 0, fmt.Errorf("unsupported dense distance %q", distance)
	}
}

func encodePoint(point contextindex.IndexedPoint) *qdrantapi.PointStruct {
	metadata := point.Metadata
	payload := map[string]any{
		"platform":         metadata.Platform,
		"profile_id":       metadata.Ref.ProfileID,
		"index_generation": metadata.Ref.IndexGeneration,
		"scope_kind":       string(metadata.ScopeKind),
		"source_kind":      string(metadata.Ref.SourceKind),
		"source_id":        metadata.Ref.SourceID,
		"source_sha256":    hex.EncodeToString(metadata.Ref.SourceSHA256[:]),
	}
	addPositive(payload, "space_id", metadata.SpaceID)
	addPositive(payload, "conversation_id", metadata.ConversationID)
	addPositive(payload, "user_id", metadata.UserID)
	addPositive(payload, "document_id", metadata.DocumentID)
	addPositive(payload, "document_version_id", metadata.DocumentVersionID)
	addPositive(payload, "chunk_id", metadata.ChunkID)

	vectors := map[string]*qdrantapi.Vector{
		denseVectorName: qdrantapi.NewVectorDense(point.Dense),
	}
	if !point.Sparse.Empty() {
		vectors[sparseVectorName] = qdrantapi.NewVectorSparse(point.Sparse.Indices, point.Sparse.Values)
	}
	return &qdrantapi.PointStruct{
		Id:      qdrantapi.NewID(metadata.Ref.ID.String()),
		Payload: qdrantapi.NewValueMap(payload),
		Vectors: qdrantapi.NewVectorsMap(vectors),
	}
}

func addPositive(payload map[string]any, key string, value uint64) {
	if value != 0 {
		payload[key] = value
	}
}

func independentQuery(collection string, branch *qdrantapi.PrefetchQuery) *qdrantapi.QueryPoints {
	return &qdrantapi.QueryPoints{
		CollectionName: collection,
		Query:          branch.Query,
		Using:          branch.Using,
		Filter:         branch.Filter,
		ScoreThreshold: branch.ScoreThreshold,
		Limit:          branch.Limit,
		WithPayload:    pointPayloadSelector(),
	}
}

func pointPayloadSelector() *qdrantapi.WithPayloadSelector {
	return qdrantapi.NewWithPayloadInclude(pointPayloadFields...)
}

func encodeScopeFilter(scope contextindex.ScopeFilter) *qdrantapi.Filter {
	branches := make([]*qdrantapi.Condition, 0, 2)
	if len(scope.SpaceIDs) > 0 {
		spaceIDs := make([]int64, len(scope.SpaceIDs))
		for i, id := range scope.SpaceIDs {
			spaceIDs[i] = int64(id)
		}
		branches = append(branches, qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchKeyword("scope_kind", string(contextindex.ScopeKindSpace)),
			qdrantapi.NewMatchKeyword("platform", scope.Platform),
			qdrantapi.NewMatchInts("space_id", spaceIDs...),
		}}))
	}
	if scope.Conversation != nil {
		branches = append(branches, qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchKeyword("scope_kind", string(contextindex.ScopeKindConversation)),
			qdrantapi.NewMatchKeyword("platform", scope.Platform),
			qdrantapi.NewMatchInt("conversation_id", int64(scope.Conversation.ConversationID)),
			qdrantapi.NewMatchInt("user_id", int64(scope.Conversation.UserID)),
		}}))
	}
	return &qdrantapi.Filter{Must: []*qdrantapi.Condition{
		qdrantapi.NewMatchInt("profile_id", int64(scope.ProfileID)),
		qdrantapi.NewMatchInt("index_generation", int64(scope.IndexGeneration)),
		qdrantapi.NewFilterAsCondition(&qdrantapi.Filter{Should: branches}),
	}}
}

func decodeScoredPoints(points []*qdrantapi.ScoredPoint) ([]contextindex.ScoredPoint, error) {
	decoded := make([]contextindex.ScoredPoint, len(points))
	for i, point := range points {
		if point == nil || point.Id == nil {
			return nil, fmt.Errorf("point %d has no ID", i)
		}
		if math.IsNaN(float64(point.Score)) || math.IsInf(float64(point.Score), 0) {
			return nil, fmt.Errorf("point %d has non-finite score", i)
		}
		metadata, err := decodeMetadata(point.Id.GetUuid(), point.Payload)
		if err != nil {
			return nil, fmt.Errorf("point %d: %w", i, err)
		}
		decoded[i] = contextindex.ScoredPoint{Metadata: metadata, Score: point.Score}
	}
	return decoded, nil
}

func decodeMetadata(idValue string, payload map[string]*qdrantapi.Value) (contextindex.PointMetadata, error) {
	if err := rejectUnknownPayload(payload); err != nil {
		return contextindex.PointMetadata{}, err
	}
	id, err := uuid.Parse(idValue)
	if err != nil {
		return contextindex.PointMetadata{}, fmt.Errorf("parse point UUID: %w", err)
	}
	profileID, err := requiredUint(payload, "profile_id")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	generation, err := requiredUint(payload, "index_generation")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	sourceID, err := requiredUint(payload, "source_id")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	sourceHashText, err := requiredString(payload, "source_sha256")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	sourceHashBytes, err := hex.DecodeString(sourceHashText)
	if err != nil || len(sourceHashBytes) != 32 || hex.EncodeToString(sourceHashBytes) != sourceHashText {
		return contextindex.PointMetadata{}, errors.New("source_sha256 must be lowercase SHA-256 hex")
	}
	var sourceHash [32]byte
	copy(sourceHash[:], sourceHashBytes)
	sourceKindText, err := requiredString(payload, "source_kind")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	ref, err := contextindex.NewPointRef(id, profileID, generation, contextindex.SourceKind(sourceKindText), sourceID, sourceHash)
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	platform, err := requiredString(payload, "platform")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	scopeKind, err := requiredString(payload, "scope_kind")
	if err != nil {
		return contextindex.PointMetadata{}, err
	}
	metadata := contextindex.PointMetadata{
		Ref:       ref,
		Platform:  platform,
		ScopeKind: contextindex.ScopeKind(scopeKind),
	}
	optionalFields := []struct {
		key    string
		target *uint64
	}{
		{key: "space_id", target: &metadata.SpaceID},
		{key: "conversation_id", target: &metadata.ConversationID},
		{key: "user_id", target: &metadata.UserID},
		{key: "document_id", target: &metadata.DocumentID},
		{key: "document_version_id", target: &metadata.DocumentVersionID},
		{key: "chunk_id", target: &metadata.ChunkID},
	}
	for _, field := range optionalFields {
		*field.target, err = optionalUint(payload, field.key)
		if err != nil {
			return contextindex.PointMetadata{}, err
		}
	}
	if err := metadata.Validate(); err != nil {
		return contextindex.PointMetadata{}, err
	}
	return metadata, nil
}

func rejectUnknownPayload(payload map[string]*qdrantapi.Value) error {
	for key := range payload {
		switch key {
		case "platform", "profile_id", "index_generation", "scope_kind", "source_kind", "source_id",
			"space_id", "conversation_id", "user_id", "document_id", "document_version_id", "chunk_id", "source_sha256":
		default:
			return fmt.Errorf("unsupported point payload field %q", key)
		}
	}
	return nil
}

func requiredUint(payload map[string]*qdrantapi.Value, key string) (uint64, error) {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, fmt.Errorf("point payload %q is required", key)
	}
	integer, ok := value.Kind.(*qdrantapi.Value_IntegerValue)
	if !ok || integer.IntegerValue <= 0 {
		return 0, fmt.Errorf("point payload %q must be a positive integer", key)
	}
	return uint64(integer.IntegerValue), nil
}

func optionalUint(payload map[string]*qdrantapi.Value, key string) (uint64, error) {
	value, ok := payload[key]
	if !ok {
		return 0, nil
	}
	if value == nil {
		return 0, fmt.Errorf("point payload %q must be a positive integer", key)
	}
	integer, ok := value.Kind.(*qdrantapi.Value_IntegerValue)
	if !ok || integer.IntegerValue <= 0 {
		return 0, fmt.Errorf("point payload %q must be a positive integer", key)
	}
	return uint64(integer.IntegerValue), nil
}

func requiredString(payload map[string]*qdrantapi.Value, key string) (string, error) {
	value, ok := payload[key]
	if !ok || value == nil {
		return "", fmt.Errorf("point payload %q is required", key)
	}
	text, ok := value.Kind.(*qdrantapi.Value_StringValue)
	if !ok || text.StringValue == "" {
		return "", fmt.Errorf("point payload %q must be a non-empty string", key)
	}
	return text.StringValue, nil
}

func stringPointer(value string) *string {
	return &value
}
