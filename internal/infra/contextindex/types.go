package contextindex

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

var platformPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,48}$`)
var collectionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,254}$`)

const maxQdrantInteger = uint64(math.MaxInt64)

type Distance string

const (
	DistanceCosine Distance = "cosine"
	DistanceDot    Distance = "dot"
	DistanceEuclid Distance = "euclid"
)

func (distance Distance) Valid() bool {
	return distance == DistanceCosine || distance == DistanceDot || distance == DistanceEuclid
}

type CollectionSpec struct {
	Name            string
	DenseDimensions uint64
	DenseDistance   Distance
}

func NewCollectionSpec(name string, denseDimensions uint64, denseDistance Distance) (CollectionSpec, error) {
	spec := CollectionSpec{
		Name:            strings.TrimSpace(name),
		DenseDimensions: denseDimensions,
		DenseDistance:   denseDistance,
	}
	if err := spec.Validate(); err != nil {
		return CollectionSpec{}, err
	}
	return spec, nil
}

func (spec CollectionSpec) Validate() error {
	if err := ValidateCollectionName(spec.Name); err != nil {
		return err
	}
	if spec.DenseDimensions == 0 {
		return errors.New("collection dense dimensions must be positive")
	}
	if !spec.DenseDistance.Valid() {
		return fmt.Errorf("unsupported dense distance %q", spec.DenseDistance)
	}
	return nil
}

func ValidateCollectionName(name string) error {
	if !collectionPattern.MatchString(name) {
		return fmt.Errorf("invalid collection name %q", name)
	}
	return nil
}

type SourceKind string

const (
	SourceKindDocumentChunk    SourceKind = "document_chunk"
	SourceKindConversationTurn SourceKind = "conversation_turn"
)

func (kind SourceKind) Valid() bool {
	return kind == SourceKindDocumentChunk || kind == SourceKindConversationTurn
}

type PointRef struct {
	ID              uuid.UUID
	ProfileID       uint64
	IndexGeneration uint64
	SourceKind      SourceKind
	SourceID        uint64
	SourceSHA256    [32]byte
}

func NewPointRef(
	id uuid.UUID,
	profileID uint64,
	indexGeneration uint64,
	sourceKind SourceKind,
	sourceID uint64,
	sourceSHA256 [32]byte,
) (PointRef, error) {
	ref := PointRef{
		ID:              id,
		ProfileID:       profileID,
		IndexGeneration: indexGeneration,
		SourceKind:      sourceKind,
		SourceID:        sourceID,
		SourceSHA256:    sourceSHA256,
	}
	if err := ref.Validate(); err != nil {
		return PointRef{}, err
	}
	return ref, nil
}

func (ref PointRef) Validate() error {
	if ref.ID.Version() != 8 || ref.ID.Variant() != uuid.RFC4122 {
		return errors.New("point ID must be an RFC 9562 UUIDv8")
	}
	if !validQdrantID(ref.ProfileID) {
		return errors.New("point profile ID must fit a positive Qdrant integer")
	}
	if !validQdrantID(ref.IndexGeneration) {
		return errors.New("point index generation must fit a positive Qdrant integer")
	}
	if !ref.SourceKind.Valid() {
		return fmt.Errorf("unsupported point source kind %q", ref.SourceKind)
	}
	if !validQdrantID(ref.SourceID) {
		return errors.New("point source ID must fit a positive Qdrant integer")
	}
	if ref.SourceSHA256 == ([32]byte{}) {
		return errors.New("point source SHA-256 must be present")
	}
	return nil
}

type ConversationScope struct {
	UserID         uint64
	ConversationID uint64
}

func (scope ConversationScope) Validate() error {
	if !validQdrantID(scope.UserID) || !validQdrantID(scope.ConversationID) {
		return errors.New("conversation user and conversation IDs must fit positive Qdrant integers")
	}
	return nil
}

type ScopeFilter struct {
	ProfileID       uint64
	IndexGeneration uint64
	Platform        string
	SpaceIDs        []uint64
	Conversation    *ConversationScope
}

func NewScopeFilter(
	profileID uint64,
	indexGeneration uint64,
	platform string,
	spaceIDs []uint64,
	conversation *ConversationScope,
) (ScopeFilter, error) {
	normalizedSpaces := slices.Clone(spaceIDs)
	slices.Sort(normalizedSpaces)
	normalizedSpaces = slices.Compact(normalizedSpaces)

	var normalizedConversation *ConversationScope
	if conversation != nil {
		copy := *conversation
		normalizedConversation = &copy
	}
	filter := ScopeFilter{
		ProfileID:       profileID,
		IndexGeneration: indexGeneration,
		Platform:        strings.ToLower(strings.TrimSpace(platform)),
		SpaceIDs:        normalizedSpaces,
		Conversation:    normalizedConversation,
	}
	if err := filter.Validate(); err != nil {
		return ScopeFilter{}, err
	}
	return filter, nil
}

func (filter ScopeFilter) Validate() error {
	if !validQdrantID(filter.ProfileID) {
		return errors.New("scope profile ID must fit a positive Qdrant integer")
	}
	if !validQdrantID(filter.IndexGeneration) {
		return errors.New("scope index generation must fit a positive Qdrant integer")
	}
	if !platformPattern.MatchString(filter.Platform) {
		return fmt.Errorf("invalid normalized platform %q", filter.Platform)
	}
	for i, id := range filter.SpaceIDs {
		if !validQdrantID(id) {
			return errors.New("scope space IDs must fit positive Qdrant integers")
		}
		if i > 0 && filter.SpaceIDs[i-1] >= id {
			return errors.New("scope space IDs must be sorted and unique")
		}
	}
	if filter.Conversation != nil {
		if err := filter.Conversation.Validate(); err != nil {
			return err
		}
	}
	if len(filter.SpaceIDs) == 0 && filter.Conversation == nil {
		return errors.New("scope must include a space or conversation branch")
	}
	return nil
}

func validQdrantID(id uint64) bool {
	return id > 0 && id <= maxQdrantInteger
}

type SparseVector struct {
	Indices []uint32
	Values  []float32
}

func (vector SparseVector) Empty() bool {
	return len(vector.Indices) == 0 && len(vector.Values) == 0
}

func NewSparseVector(indices []uint32, values []float32) (SparseVector, error) {
	vector := SparseVector{
		Indices: slices.Clone(indices),
		Values:  slices.Clone(values),
	}
	if err := vector.Validate(); err != nil {
		return SparseVector{}, err
	}
	return vector, nil
}

func (vector SparseVector) Validate() error {
	if len(vector.Indices) != len(vector.Values) {
		return errors.New("sparse indices and values must have equal lengths")
	}
	for i, value := range vector.Values {
		if value <= 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("sparse value %d must be finite and positive", i)
		}
		if i > 0 && vector.Indices[i-1] >= vector.Indices[i] {
			return errors.New("sparse indices must be strictly ascending and unique")
		}
	}
	return nil
}

type ScopeKind string

const (
	ScopeKindSpace        ScopeKind = "space"
	ScopeKindConversation ScopeKind = "conversation"
)

type PointMetadata struct {
	Ref               PointRef
	Platform          string
	ScopeKind         ScopeKind
	SpaceID           uint64
	ConversationID    uint64
	UserID            uint64
	DocumentID        uint64
	DocumentVersionID uint64
	ChunkID           uint64
}

func (metadata PointMetadata) Validate() error {
	if err := metadata.Ref.Validate(); err != nil {
		return err
	}
	if !platformPattern.MatchString(metadata.Platform) {
		return fmt.Errorf("invalid normalized platform %q", metadata.Platform)
	}
	switch metadata.ScopeKind {
	case ScopeKindSpace:
		if !validQdrantID(metadata.SpaceID) || metadata.ConversationID != 0 || metadata.UserID != 0 {
			return errors.New("space point must have only a positive space ID")
		}
	case ScopeKindConversation:
		if metadata.SpaceID != 0 || !validQdrantID(metadata.ConversationID) || !validQdrantID(metadata.UserID) {
			return errors.New("conversation point must have paired user and conversation IDs")
		}
	default:
		return fmt.Errorf("unsupported point scope kind %q", metadata.ScopeKind)
	}

	switch metadata.Ref.SourceKind {
	case SourceKindDocumentChunk:
		if !validQdrantID(metadata.DocumentID) || !validQdrantID(metadata.DocumentVersionID) || !validQdrantID(metadata.ChunkID) {
			return errors.New("document point must have document, version and chunk IDs")
		}
		if metadata.ChunkID != metadata.Ref.SourceID {
			return errors.New("document point source ID must equal chunk ID")
		}
	case SourceKindConversationTurn:
		if metadata.ScopeKind != ScopeKindConversation {
			return errors.New("conversation turn point must use conversation scope")
		}
		if metadata.DocumentID != 0 || metadata.DocumentVersionID != 0 || metadata.ChunkID != 0 {
			return errors.New("conversation turn point cannot have document identity")
		}
	}
	return nil
}

type IndexedPoint struct {
	Metadata PointMetadata
	Dense    []float32
	Sparse   SparseVector
}

func NewIndexedPoint(metadata PointMetadata, dense []float32, sparse SparseVector) (IndexedPoint, error) {
	point := IndexedPoint{
		Metadata: metadata,
		Dense:    slices.Clone(dense),
		Sparse: SparseVector{
			Indices: slices.Clone(sparse.Indices),
			Values:  slices.Clone(sparse.Values),
		},
	}
	if err := point.Validate(); err != nil {
		return IndexedPoint{}, err
	}
	return point, nil
}

func (point IndexedPoint) Validate() error {
	if err := point.Metadata.Validate(); err != nil {
		return err
	}
	if err := validateDense(point.Dense); err != nil {
		return err
	}
	return point.Sparse.Validate()
}

type QueryVariant struct {
	Dense  []float32
	Sparse SparseVector
}

type HybridQuery struct {
	Collection string
	Filter     ScopeFilter
	Variants   []QueryVariant
	Limit      uint64
}

func NewHybridQuery(collection string, filter ScopeFilter, variants []QueryVariant, limit uint64) (HybridQuery, error) {
	clonedFilter := filter
	clonedFilter.SpaceIDs = slices.Clone(filter.SpaceIDs)
	if filter.Conversation != nil {
		conversation := *filter.Conversation
		clonedFilter.Conversation = &conversation
	}
	clonedVariants := make([]QueryVariant, len(variants))
	for i, variant := range variants {
		clonedVariants[i] = QueryVariant{
			Dense: slices.Clone(variant.Dense),
			Sparse: SparseVector{
				Indices: slices.Clone(variant.Sparse.Indices),
				Values:  slices.Clone(variant.Sparse.Values),
			},
		}
	}
	request := HybridQuery{
		Collection: strings.TrimSpace(collection),
		Filter:     clonedFilter,
		Variants:   clonedVariants,
		Limit:      limit,
	}
	if err := request.Validate(); err != nil {
		return HybridQuery{}, err
	}
	return request, nil
}

func (request HybridQuery) Validate() error {
	if !collectionPattern.MatchString(request.Collection) {
		return fmt.Errorf("invalid collection name %q", request.Collection)
	}
	if err := request.Filter.Validate(); err != nil {
		return err
	}
	if len(request.Variants) == 0 {
		return errors.New("hybrid query must have at least one variant")
	}
	if request.Limit == 0 || request.Limit > 200 {
		return errors.New("hybrid query limit must be between 1 and 200")
	}
	for i, variant := range request.Variants {
		if err := validateDense(variant.Dense); err != nil {
			return fmt.Errorf("query variant %d: %w", i, err)
		}
		if err := variant.Sparse.Validate(); err != nil {
			return fmt.Errorf("query variant %d: %w", i, err)
		}
	}
	return nil
}

func validateDense(vector []float32) error {
	if len(vector) == 0 {
		return errors.New("dense vector must not be empty")
	}
	for i, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("dense value %d must be finite", i)
		}
	}
	return nil
}

type Modality string

const (
	ModalityDense  Modality = "dense"
	ModalitySparse Modality = "sparse"
)

type ScoredPoint struct {
	Metadata PointMetadata
	Score    float32
}

type BranchResult struct {
	VariantIndex int
	Modality     Modality
	Points       []ScoredPoint
}

type HybridResult struct {
	Branches []BranchResult
	Fused    []ScoredPoint
}
