package contextengine

import (
	"crypto/sha256"
	"testing"

	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
)

func TestPointIDIsStableUUIDv8(t *testing.T) {
	sourceSHA := sha256.Sum256([]byte("facts"))
	id, err := PointID(7, contextindex.SourceKindDocumentChunk, 91, sourceSHA)
	if err != nil {
		t.Fatal(err)
	}
	if id.Version() != 8 || id.Variant() != uuid.RFC4122 {
		t.Fatalf("id=%s version=%d variant=%v", id, id.Version(), id.Variant())
	}
	again, err := PointID(7, contextindex.SourceKindDocumentChunk, 91, sourceSHA)
	if err != nil || id != again {
		t.Fatalf("same source produced %s then %s, err=%v", id, again, err)
	}
}

func TestPointIDRejectsInvalidSourceFacts(t *testing.T) {
	if _, err := PointID(0, contextindex.SourceKindDocumentChunk, 91, sha256.Sum256([]byte("facts"))); err == nil {
		t.Fatal("zero profile ID was accepted")
	}
	if _, err := PointID(7, contextindex.SourceKind("unknown"), 91, sha256.Sum256([]byte("facts"))); err == nil {
		t.Fatal("unknown source kind was accepted")
	}
}
