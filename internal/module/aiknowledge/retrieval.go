package aiknowledge

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var latinTermPattern = regexp.MustCompile(`[A-Za-z0-9_]+`)
var hanTermPattern = regexp.MustCompile(`\p{Han}+`)

var chineseStopWords = []string{"为什么", "是什么", "是不是", "怎么样", "怎么办", "如何", "哪些", "哪个", "什么", "问题", "原因", "情况", "不应该", "应该", "需要", "可以", "不能", "只有", "一个", "一种", "这个", "那个", "以及", "还是", "或者", "并且", "如果", "那么", "要做", "怎么", "多少", "的是", "要", "做", "的", "了", "吗", "呢", "啊", "吧"}

type RetrievalChunk struct {
	KnowledgeBaseID int64   `json:"knowledge_base_id"`
	DocumentID      int64   `json:"document_id"`
	DocumentTitle   string  `json:"document_title"`
	ChunkNo         int     `json:"chunk_no"`
	Content         string  `json:"content"`
	Score           float64 `json:"score"`
}

func ScoreKeywordChunk(content string, query string) float64 {
	content = strings.TrimSpace(content)
	terms := QueryTerms(query)
	if content == "" || len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(content)
	score := 0.0
	for _, term := range terms {
		count := strings.Count(lower, strings.ToLower(term))
		if count <= 0 {
			continue
		}
		score += float64(count)
		if utf8.RuneCountInString(term) >= 2 {
			score += 0.25
		}
	}
	return score
}

func BuildContextPrompt(chunks []RetrievalChunk) string {
	if len(chunks) == 0 {
		return ""
	}
	lines := []string{"以下是可参考的知识库片段。回答时优先依据这些片段；如果片段不足以回答，请明确说明不确定，不要编造。"}
	for i, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		title := strings.TrimSpace(chunk.DocumentTitle)
		if title == "" {
			title = "未命名文档"
		}
		chunkNo := chunk.ChunkNo
		if chunkNo <= 0 {
			chunkNo = i + 1
		}
		lines = append(lines, "", fmt.Sprintf("[%d] 来源：%s #%d，score=%.2f", i+1, title, chunkNo, math.Round(chunk.Score*100)/100), content)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func RankChunks(candidates []RetrievalChunk, query string, topK int, threshold float64) []RetrievalChunk {
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}
	ranked := make([]RetrievalChunk, 0, len(candidates))
	for _, item := range candidates {
		item.Score = ScoreKeywordChunk(item.Content, query)
		if item.Score >= threshold {
			ranked = append(ranked, item)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].DocumentID > ranked[j].DocumentID
		}
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	return ranked
}

func QueryTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	terms := []string{}
	for _, term := range latinTermPattern.FindAllString(query, -1) {
		if usefulTerm(term) {
			terms = append(terms, term)
		}
	}
	for _, text := range hanTermPattern.FindAllString(query, -1) {
		terms = append(terms, chineseQueryTerms(text)...)
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if !usefulTerm(term) {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

func chineseQueryTerms(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	terms := []string{}
	if utf8.RuneCountInString(text) <= 12 {
		terms = append(terms, text)
	}
	cleaned := text
	for _, stop := range chineseStopWords {
		cleaned = strings.ReplaceAll(cleaned, stop, "")
	}
	if cleaned == "" {
		return terms
	}
	if cleaned != text {
		terms = append(terms, cleaned)
	}
	runes := []rune(cleaned)
	for _, size := range []int{4, 3, 2} {
		if len(runes) < size {
			continue
		}
		for offset := 0; offset <= len(runes)-size; offset++ {
			terms = append(terms, string(runes[offset:offset+size]))
		}
	}
	return terms
}
func usefulTerm(term string) bool {
	term = strings.TrimSpace(term)
	if utf8.RuneCountInString(term) < 2 {
		return false
	}
	for _, stop := range chineseStopWords {
		if term == stop {
			return false
		}
	}
	return true
}
