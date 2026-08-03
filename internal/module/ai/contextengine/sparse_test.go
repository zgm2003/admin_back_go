package contextengine

import (
	"math"
	"reflect"
	"testing"

	"admin_back_go/internal/infra/contextindex"
)

func TestUnicodeLexicalV1Golden(t *testing.T) {
	got, err := EncodeSparse("Go语言 GO 语言123")
	if err != nil {
		t.Fatal(err)
	}
	want := contextindex.SparseVector{
		Indices: []uint32{701548806, 1708916009, 2415828576, 2669990252, 4154103862},
		Values:  []float32{1.6931472, 1.6931472, 1.6931472, 1.6931472, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EncodeSparse()=%#v want=%#v", got, want)
	}
}

func TestSparseEncoderAggregatesHashCollisions(t *testing.T) {
	got, err := encodeSparseWithIndexer("alpha beta alpha", func(string) uint32 { return 9 })
	if err != nil {
		t.Fatal(err)
	}
	want := float32((1 + math.Log(2)) + 1)
	if len(got.Indices) != 1 || got.Indices[0] != 9 || got.Values[0] != want {
		t.Fatalf("collision vector=%#v want index 9 value %v", got, want)
	}
}

func TestUnicodeLexicalV1NormalizesCompatibilityAndCase(t *testing.T) {
	left, err := EncodeSparse("ＧＯ")
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeSparse("go")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("NFKC/case folding mismatch: %#v != %#v", left, right)
	}
}
