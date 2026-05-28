package aiknowledge

import (
	"strings"
	"testing"
)

func TestSelectHitsAppliesScoreAndContextLimit(t *testing.T) {
	candidates := []RetrievalCandidate{
		{KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 10, DocumentTitle: "Go 后端架构", ChunkID: 100, ChunkIndex: 1, Title: "Go 后端架构", Content: "admin_back_go 采用 Gin modular monolith，调用链是 route handler service repository model。", ContentChars: 50},
		{KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 11, DocumentTitle: "无关", ChunkID: 101, ChunkIndex: 1, Title: "无关", Content: "天气很好。", ContentChars: 5},
		{KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 12, DocumentTitle: "调用链", ChunkID: 102, ChunkIndex: 1, Title: "调用链", Content: "route service 都属于后端调用链。", ContentChars: 50},
	}
	result := SelectHits("Go 后端架构 route service", candidates, RetrievalOptions{TopK: 5, MinScore: 0.1, MaxContextChars: 60})
	if result.TotalHits != 3 || result.SelectedHits != 1 {
		t.Fatalf("unexpected totals: %#v", result)
	}
	if len(result.Hits) != 3 {
		t.Fatalf("hit count = %d", len(result.Hits))
	}
	if result.Hits[0].Status != HitStatusSelected || result.Hits[0].RankNo != 1 || result.Hits[0].Score <= 0 {
		t.Fatalf("first hit not selected with score: %#v", result.Hits[0])
	}
	if result.Hits[1].Status != HitStatusSkipped || result.Hits[1].SkipReason != SkipReasonContextLimit {
		t.Fatalf("second hit should be context limit skipped: %#v", result.Hits[1])
	}
	if result.Hits[2].Status != HitStatusSkipped || result.Hits[2].SkipReason != SkipReasonLowScore {
		t.Fatalf("third hit should be low score skipped: %#v", result.Hits[2])
	}
}

func TestBuildKnowledgeContextUsesSelectedHitsOnly(t *testing.T) {
	contextText := BuildKnowledgeContext([]SelectedHit{
		{Ref: "K1", KnowledgeBaseName: "架构库", DocumentTitle: "Go 后端架构", ChunkIndex: 1, Content: "Gin modular monolith"},
	})
	for _, part := range []string{"[K1]", "架构库", "Go 后端架构", "Gin modular monolith"} {
		if !strings.Contains(contextText, part) {
			t.Fatalf("context missing %q: %q", part, contextText)
		}
	}
	if strings.Contains(contextText, "用户问题") {
		t.Fatalf("context builder should only build selected knowledge snippets: %q", contextText)
	}
	if BuildKnowledgeContext(nil) != "" {
		t.Fatal("empty selected hits should build empty context")
	}
}
