package aiknowledge

import (
	"strings"
	"testing"
)

func TestChunkTextUsesSizeAndOverlap(t *testing.T) {
	text := strings.Repeat("a", 300) + strings.Repeat("b", 300) + strings.Repeat("c", 300)
	chunks, err := chunkText(text, ChunkOptions{SizeChars: 400, OverlapChars: 100})
	if err != nil {
		t.Fatalf("chunkText returned error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3: %#v", len(chunks), chunks)
	}
	if chunks[0].Index != 1 || chunks[1].Index != 2 || chunks[2].Index != 3 {
		t.Fatalf("unexpected indexes: %#v", chunks)
	}
	if chunks[0].Chars != 400 || chunks[1].Chars != 400 || chunks[2].Chars != 300 {
		t.Fatalf("unexpected chunk sizes: %#v", chunks)
	}
}

func TestChunkTextRejectsInvalidOptions(t *testing.T) {
	_, err := chunkText("hello", ChunkOptions{SizeChars: 10, OverlapChars: 10})
	if err == nil {
		t.Fatal("expected error for overlap >= size")
	}
}
