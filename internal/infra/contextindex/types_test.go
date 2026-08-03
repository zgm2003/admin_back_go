package contextindex

import (
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestNewScopeFilterNormalizesClosedScope(t *testing.T) {
	filter, err := NewScopeFilter(7, 3, " Admin ", []uint64{19, 11, 19}, &ConversationScope{
		UserID:         23,
		ConversationID: 29,
	})
	if err != nil {
		t.Fatal(err)
	}

	if filter.Platform != "admin" {
		t.Fatalf("platform=%q", filter.Platform)
	}
	if !reflect.DeepEqual(filter.SpaceIDs, []uint64{11, 19}) {
		t.Fatalf("space_ids=%v", filter.SpaceIDs)
	}
	if filter.Conversation == nil || filter.Conversation.UserID != 23 || filter.Conversation.ConversationID != 29 {
		t.Fatalf("conversation=%+v", filter.Conversation)
	}
}

func TestCollectionSpecAndIndexedPointRejectOpenState(t *testing.T) {
	spec, err := NewCollectionSpec("admin_context_profile_7_g3", 4, DistanceCosine)
	if err != nil {
		t.Fatal(err)
	}
	if spec.DenseDimensions != 4 || spec.DenseDistance != DistanceCosine {
		t.Fatalf("spec=%+v", spec)
	}

	ref := mustTestPointRef(t, SourceKindDocumentChunk, 41)
	point, err := NewIndexedPoint(PointMetadata{
		Ref:               ref,
		Platform:          "admin",
		ScopeKind:         ScopeKindSpace,
		SpaceID:           11,
		DocumentID:        31,
		DocumentVersionID: 37,
		ChunkID:           41,
	}, []float32{1, 0, 0, 0}, SparseVector{})
	if err != nil {
		t.Fatal(err)
	}
	if point.Metadata.Ref != ref || len(point.Dense) != 4 {
		t.Fatalf("point=%+v", point)
	}

	invalid := []IndexedPoint{
		{Metadata: point.Metadata, Dense: nil},
		{Metadata: PointMetadata{Ref: ref, Platform: "admin", ScopeKind: ScopeKindSpace, DocumentID: 31, DocumentVersionID: 37, ChunkID: 41}, Dense: []float32{1}},
		{Metadata: PointMetadata{Ref: ref, Platform: "admin", ScopeKind: ScopeKindConversation, UserID: 9, ConversationID: 10, DocumentID: 31, DocumentVersionID: 37, ChunkID: 40}, Dense: []float32{1}},
		{Metadata: PointMetadata{Ref: mustTestPointRef(t, SourceKindConversationTurn, 41), Platform: "admin", ScopeKind: ScopeKindSpace, SpaceID: 11}, Dense: []float32{1}},
	}
	for i, candidate := range invalid {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid point %d accepted", i)
		}
	}
}

func TestHybridQueryRequiresDenseAndOmitsEmptySparse(t *testing.T) {
	filter, err := NewScopeFilter(7, 3, "admin", []uint64{11}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewHybridQuery("admin_context_profile_7_g3", filter, []QueryVariant{
		{Dense: []float32{1, 0, 0, 0}},
		{Dense: []float32{0, 1, 0, 0}, Sparse: SparseVector{Indices: []uint32{9}, Values: []float32{1}}},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Variants) != 2 || !request.Variants[0].Sparse.Empty() {
		t.Fatalf("request=%+v", request)
	}
	filter.SpaceIDs[0] = 99
	if request.Filter.SpaceIDs[0] != 11 {
		t.Fatalf("request scope changed through caller alias: %v", request.Filter.SpaceIDs)
	}

	request.Variants[0].Dense = nil
	if err := request.Validate(); err == nil {
		t.Fatal("missing dense vector accepted")
	}
}

func mustTestPointRef(t *testing.T, kind SourceKind, sourceID uint64) PointRef {
	t.Helper()
	ref, err := NewPointRef(
		uuid.MustParse("018f3f5e-7b4c-8123-8abc-0123456789ab"),
		7,
		3,
		kind,
		sourceID,
		[32]byte{1},
	)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestNewScopeFilterRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name         string
		profileID    uint64
		generation   uint64
		platform     string
		spaceIDs     []uint64
		conversation *ConversationScope
	}{
		{name: "missing profile", generation: 1, platform: "admin", spaceIDs: []uint64{1}},
		{name: "missing generation", profileID: 1, platform: "admin", spaceIDs: []uint64{1}},
		{name: "invalid platform", profileID: 1, generation: 1, platform: "admin user", spaceIDs: []uint64{1}},
		{name: "zero space", profileID: 1, generation: 1, platform: "admin", spaceIDs: []uint64{0}},
		{name: "unpaired conversation user", profileID: 1, generation: 1, platform: "admin", conversation: &ConversationScope{ConversationID: 2}},
		{name: "unpaired conversation id", profileID: 1, generation: 1, platform: "admin", conversation: &ConversationScope{UserID: 2}},
		{name: "no branch", profileID: 1, generation: 1, platform: "admin"},
		{name: "profile over qdrant integer", profileID: math.MaxUint64, generation: 1, platform: "admin", spaceIDs: []uint64{1}},
		{name: "space over qdrant integer", profileID: 1, generation: 1, platform: "admin", spaceIDs: []uint64{math.MaxUint64}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewScopeFilter(tt.profileID, tt.generation, tt.platform, tt.spaceIDs, tt.conversation); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewSparseVectorAcceptsEmptyAndStrictCoordinates(t *testing.T) {
	empty, err := NewSparseVector(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Indices) != 0 || len(empty.Values) != 0 {
		t.Fatalf("empty=%+v", empty)
	}

	vector, err := NewSparseVector([]uint32{3, 8}, []float32{1, 2.5})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(vector.Indices, []uint32{3, 8}) || !reflect.DeepEqual(vector.Values, []float32{1, 2.5}) {
		t.Fatalf("vector=%+v", vector)
	}
}

func TestNewSparseVectorRejectsInvalidCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		indices []uint32
		values  []float32
	}{
		{name: "length mismatch", indices: []uint32{1}, values: nil},
		{name: "duplicate", indices: []uint32{1, 1}, values: []float32{1, 2}},
		{name: "descending", indices: []uint32{2, 1}, values: []float32{1, 2}},
		{name: "zero weight", indices: []uint32{1}, values: []float32{0}},
		{name: "negative weight", indices: []uint32{1}, values: []float32{-1}},
		{name: "infinite weight", indices: []uint32{1}, values: []float32{float32(math.Inf(1))}},
		{name: "nan weight", indices: []uint32{1}, values: []float32{float32(math.NaN())}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSparseVector(tt.indices, tt.values); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewPointRefRequiresClosedSourceAndUUIDv8(t *testing.T) {
	id := uuid.MustParse("018f3f5e-7b4c-8123-8abc-0123456789ab")
	ref, err := NewPointRef(id, 7, 3, SourceKindDocumentChunk, 11, [32]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != id || ref.SourceKind != SourceKindDocumentChunk {
		t.Fatalf("ref=%+v", ref)
	}

	invalid := []struct {
		name       string
		id         uuid.UUID
		profileID  uint64
		generation uint64
		sourceKind SourceKind
		sourceID   uint64
		hash       [32]byte
	}{
		{name: "uuid version", id: uuid.New(), profileID: 7, generation: 3, sourceKind: SourceKindDocumentChunk, sourceID: 11, hash: [32]byte{1}},
		{name: "profile", id: id, generation: 3, sourceKind: SourceKindDocumentChunk, sourceID: 11, hash: [32]byte{1}},
		{name: "generation", id: id, profileID: 7, sourceKind: SourceKindDocumentChunk, sourceID: 11, hash: [32]byte{1}},
		{name: "source kind", id: id, profileID: 7, generation: 3, sourceKind: "file", sourceID: 11, hash: [32]byte{1}},
		{name: "source id", id: id, profileID: 7, generation: 3, sourceKind: SourceKindDocumentChunk, hash: [32]byte{1}},
		{name: "source hash", id: id, profileID: 7, generation: 3, sourceKind: SourceKindDocumentChunk, sourceID: 11},
		{name: "source over qdrant integer", id: id, profileID: 7, generation: 3, sourceKind: SourceKindDocumentChunk, sourceID: math.MaxUint64, hash: [32]byte{1}},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewPointRef(tt.id, tt.profileID, tt.generation, tt.sourceKind, tt.sourceID, tt.hash); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
