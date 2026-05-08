package aiknowledge

import (
	"math"
	"strings"
	"unicode/utf8"
)

type ChunkPayload struct {
	Content       string         `json:"content"`
	TokenEstimate int            `json:"token_estimate"`
	MetadataJSON  map[string]any `json:"metadata_json"`
}

func ChunkText(text string, chunkSize int, overlap int) []ChunkPayload {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 800
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []ChunkPayload{{Content: text, TokenEstimate: TokenEstimate(text), MetadataJSON: map[string]any{"source": "mysql_keyword"}}}
	}
	step := chunkSize - overlap
	chunks := []ChunkPayload{}
	lastEnd := 0
	for offset := 0; offset+chunkSize <= len(runes); offset += step {
		chunk := strings.TrimSpace(string(runes[offset : offset+chunkSize]))
		if chunk != "" {
			chunks = append(chunks, ChunkPayload{Content: chunk, TokenEstimate: TokenEstimate(chunk), MetadataJSON: map[string]any{"source": "mysql_keyword"}})
		}
		lastEnd = offset + chunkSize
	}
	if lastEnd < len(runes) {
		tailOffset := len(runes) - chunkSize
		if tailOffset < 0 {
			tailOffset = 0
		}
		tail := strings.TrimSpace(string(runes[tailOffset:]))
		if tail != "" && (len(chunks) == 0 || chunks[len(chunks)-1].Content != tail) {
			chunks = append(chunks, ChunkPayload{Content: tail, TokenEstimate: TokenEstimate(tail), MetadataJSON: map[string]any{"source": "mysql_keyword"}})
		}
	}
	return chunks
}

func TokenEstimate(text string) int {
	return int(math.Max(1, math.Ceil(float64(utf8.RuneCountInString(text))/2)))
}
