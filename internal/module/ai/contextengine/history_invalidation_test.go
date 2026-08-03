package contextengine

import (
	"reflect"
	"testing"
)

func TestHistoryDerivedInvalidationDeduplicatesAffectedTurnAnchors(t *testing.T) {
	anchors := []uint64{9, 3, 0, 9, 7, 3}
	if got := dedupeHistoryAnchors(anchors); !reflect.DeepEqual(got, []uint64{3, 7, 9}) {
		t.Fatalf("anchors=%v", got)
	}
}
