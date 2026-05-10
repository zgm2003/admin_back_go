package aiknowledge

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	RetrievalStatusSuccess = "success"
	RetrievalStatusFailed  = "failed"
	RetrievalStatusSkipped = "skipped"

	HitStatusSelected = 1
	HitStatusSkipped  = 2

	SkipReasonLowScore     = "low_score"
	SkipReasonContextLimit = "context_limit"
)

func SelectHits(query string, candidates []RetrievalCandidate, options RetrievalOptions) RetrievalResult {
	cleanQuery := strings.TrimSpace(query)
	result := RetrievalResult{
		Query:     cleanQuery,
		Status:    RetrievalStatusSuccess,
		TotalHits: uint(len(candidates)),
		Hits:      make([]RetrievalHit, 0, len(candidates)),
	}
	if len(candidates) == 0 || cleanQuery == "" {
		return result
	}

	scored := make([]RetrievalHit, 0, len(candidates))
	for _, candidate := range candidates {
		scored = append(scored, RetrievalHit{
			KnowledgeBaseID:   candidate.KnowledgeBaseID,
			KnowledgeBaseName: candidate.KnowledgeBaseName,
			DocumentID:        candidate.DocumentID,
			DocumentTitle:     candidate.DocumentTitle,
			ChunkID:           candidate.ChunkID,
			ChunkIndex:        candidate.ChunkIndex,
			Score:             scoreCandidate(cleanQuery, candidate),
			Content:           candidate.Content,
			ContentChars:      candidate.ContentChars,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].ChunkID < scored[j].ChunkID
		}
		return scored[i].Score > scored[j].Score
	})

	var selected uint
	var contextChars uint
	for i := range scored {
		hit := &scored[i]
		hit.RankNo = uint(i + 1)
		if hit.Score < options.MinScore {
			hit.Status = HitStatusSkipped
			hit.SkipReason = SkipReasonLowScore
			continue
		}
		if options.TopK > 0 && selected >= options.TopK {
			hit.Status = HitStatusSkipped
			hit.SkipReason = SkipReasonContextLimit
			continue
		}
		hitChars := hit.ContentChars
		if hitChars == 0 {
			hitChars = contentChars(hit.Content)
		}
		if options.MaxContextChars > 0 && contextChars+hitChars > options.MaxContextChars {
			hit.Status = HitStatusSkipped
			hit.SkipReason = SkipReasonContextLimit
			continue
		}

		selected++
		contextChars += hitChars
		hit.Status = HitStatusSelected
		result.Selected = append(result.Selected, SelectedHit{
			Ref:               fmt.Sprintf("K%d", selected),
			KnowledgeBaseID:   hit.KnowledgeBaseID,
			KnowledgeBaseName: hit.KnowledgeBaseName,
			DocumentID:        hit.DocumentID,
			DocumentTitle:     hit.DocumentTitle,
			ChunkID:           hit.ChunkID,
			ChunkIndex:        hit.ChunkIndex,
			Score:             hit.Score,
			RankNo:            hit.RankNo,
			Content:           hit.Content,
		})
	}
	result.SelectedHits = selected
	result.Hits = scored
	return result
}

func BuildKnowledgeContext(selected []SelectedHit) string {
	if len(selected) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("以下是当前智能体允许读取的知识库片段。回答时只能把这些片段当作项目内知识参考；如果片段不足，请明确说明知识库没有覆盖。")
	for i, hit := range selected {
		ref := hit.Ref
		if ref == "" {
			ref = fmt.Sprintf("K%d", i+1)
		}
		b.WriteString("\n\n[")
		b.WriteString(ref)
		b.WriteString("] 知识库：")
		b.WriteString(hit.KnowledgeBaseName)
		b.WriteString("；文档：")
		b.WriteString(hit.DocumentTitle)
		b.WriteString("；分块：")
		b.WriteString(fmt.Sprint(hit.ChunkIndex))
		b.WriteByte('\n')
		b.WriteString(hit.Content)
	}
	return b.String()
}

func scoreCandidate(query string, candidate RetrievalCandidate) float64 {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0
	}
	title := strings.ToLower(candidate.Title)
	content := strings.ToLower(candidate.Content)

	var score float64
	if strings.Contains(title, q) {
		score += 8
	}
	if strings.Contains(content, q) {
		score += 5
	}
	for _, token := range queryTokens(q) {
		if strings.Contains(title, token) {
			score += 2
		}
		if strings.Contains(content, token) {
			score += 1
		}
	}
	return score
}

func queryTokens(query string) []string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	seen := make(map[string]struct{}, len(fields))
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(strings.ToLower(field))
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

func contentChars(content string) uint {
	return uint(len([]rune(strings.TrimSpace(content))))
}
