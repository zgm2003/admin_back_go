package contextengine

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

func TestChunkerKeepsStructuralBlocksWholeAndDeterministic(t *testing.T) {
	counter, err := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	chunker, err := NewChunker(counter, 20)
	if err != nil {
		t.Fatal(err)
	}
	blocks := []StructuralBlock{
		{Ordinal: 0, Text: "first", HeadingPath: []string{"Guide"}, Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"}},
		{Ordinal: 1, Text: "second", HeadingPath: []string{"Guide"}, Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"}},
	}

	first, err := chunker.Chunk(blocks)
	if err != nil {
		t.Fatal(err)
	}
	second, err := chunker.Chunk(blocks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated chunking changed output:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 2 || first[0].Text != "first" || first[1].Text != "second" {
		t.Fatalf("structural blocks were merged or split: %#v", first)
	}
	if first[0].IndexText != "Guide\nfirst" || first[0].ContentSHA256 == ([32]byte{}) || first[0].ChunkFactsSHA256 == ([32]byte{}) {
		t.Fatalf("chunk facts are incomplete: %#v", first[0])
	}
	if first[0].EmbeddingInputTokenUpperBound != int64(len("Guide\nfirst")) {
		t.Fatalf("index text bound=%d", first[0].EmbeddingInputTokenUpperBound)
	}
}

func TestChunkerCountsHeadingPrefixInsideEmbeddingLimit(t *testing.T) {
	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	chunker, err := NewChunker(counter, 10)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunker.Chunk([]StructuralBlock{{
		Text: "abcdef", HeadingPath: []string{"Head"}, Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].IndexText != "Head\nabcde" || chunks[0].EmbeddingInputTokenUpperBound != 10 {
		t.Fatalf("heading-aware chunks=%#v", chunks)
	}
	tooLarge, _ := NewChunker(counter, 4)
	if _, err := tooLarge.Chunk([]StructuralBlock{{
		Text: "x", HeadingPath: []string{"Head"}, Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"},
	}}); !errors.Is(err, ErrChunkTokenUnitTooLarge) {
		t.Fatalf("heading overflow error=%v", err)
	}
}

func TestChunkerSplitsOnlyOversizedBlocksWithFixedOverlap(t *testing.T) {
	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	chunker, err := NewChunker(counter, 10)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunker.Chunk([]StructuralBlock{{
		Ordinal: 4, Text: "abcdefghijklmnop", Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Text != "abcdefghij" || chunks[1].Text != "jklmnop" {
		t.Fatalf("chunks=%#v want [abcdefghij jklmnop]", chunks)
	}
	if chunks[0].Ordinal != 0 || chunks[1].Ordinal != 1 || chunks[0].SourceBlockOrdinal != 4 || chunks[1].SourceBlockOrdinal != 4 {
		t.Fatalf("unexpected ordinals: %#v", chunks)
	}
}

func TestChunkerRejectsUnrepresentableTokenUnitAndChangedRetry(t *testing.T) {
	chunker, err := NewChunker(fixedUnitCounter{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = chunker.Chunk([]StructuralBlock{{Text: "x", Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"}}})
	if !errors.Is(err, ErrChunkTokenUnitTooLarge) {
		t.Fatalf("Chunk error=%v want ErrChunkTokenUnitTooLarge", err)
	}

	counter, _ := infraai.ResolveTokenCounter(infraai.TokenCounterUTF8BytesV1)
	valid, _ := NewChunker(counter, 20)
	existing, err := valid.Chunk([]StructuralBlock{{Text: "facts", Locator: ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: "paragraph"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]Chunk(nil), existing...)
	changed[0].Text = "changed"
	if err := ValidateChunkRetry(existing, changed); !errors.Is(err, ErrChunkFactsConflict) {
		t.Fatalf("ValidateChunkRetry error=%v want conflict", err)
	}
}

type fixedUnitCounter struct{}

func (fixedUnitCounter) ID() string                                    { return "fixed_test_v1" }
func (fixedUnitCounter) UpperBoundText(string) (int64, error)          { return 2, nil }
func (fixedUnitCounter) UpperBoundJSON(json.RawMessage) (int64, error) { return 2, nil }
