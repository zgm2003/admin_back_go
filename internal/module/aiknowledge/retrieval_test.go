package aiknowledge

import "testing"

func TestScoreKeywordChunkRanksUsefulTerms(t *testing.T) {
	score := ScoreKeywordChunk("知识库支持权限控制，知识库支持文档切片", "知识库 权限")
	if score <= 2 {
		t.Fatalf("expected useful score, got %v", score)
	}
	if ScoreKeywordChunk("完全无关", "知识库") != 0 {
		t.Fatalf("unrelated content must score zero")
	}
}

func TestBuildContextPromptUsesOrderedChunks(t *testing.T) {
	prompt := BuildContextPrompt([]RetrievalChunk{{DocumentTitle: "手册", ChunkNo: 2, Content: "权限说明", Score: 1.25}})
	if prompt == "" || !contains(prompt, "来源：手册 #2") || !contains(prompt, "权限说明") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func contains(s string, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s string, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
