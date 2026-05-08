package aiknowledge

import "testing"

func TestChunkTextUsesOverlapAndDedupesTail(t *testing.T) {
	chunks := ChunkText("abcdefghij", 4, 1)
	want := []string{"abcd", "defg", "ghij"}
	if len(chunks) != len(want) {
		t.Fatalf("chunks=%#v want %#v", chunks, want)
	}
	for i := range want {
		if chunks[i].Content != want[i] || chunks[i].TokenEstimate <= 0 {
			t.Fatalf("chunk %d = %#v", i, chunks[i])
		}
	}
}

func TestChunkTextReturnsEmptyForBlank(t *testing.T) {
	if chunks := ChunkText("  ", 800, 120); len(chunks) != 0 {
		t.Fatalf("expected empty chunks, got %#v", chunks)
	}
}
