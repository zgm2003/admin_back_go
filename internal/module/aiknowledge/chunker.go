package aiknowledge

import (
	"fmt"
	"strings"
)

func ChunkText(content string, options ChunkOptions) ([]TextChunk, error) {
	if options.SizeChars < 300 {
		return nil, fmt.Errorf("chunk size must be at least 300")
	}
	if options.SizeChars > 8000 {
		return nil, fmt.Errorf("chunk size must be at most 8000")
	}
	return chunkText(content, options)
}

func chunkText(content string, options ChunkOptions) ([]TextChunk, error) {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil, fmt.Errorf("content is empty")
	}
	if options.SizeChars == 0 {
		return nil, fmt.Errorf("chunk size is required")
	}
	if options.OverlapChars >= options.SizeChars {
		return nil, fmt.Errorf("chunk overlap must be smaller than chunk size")
	}

	runes := []rune(text)
	if uint(len(runes)) <= options.SizeChars {
		return []TextChunk{{Index: 1, Content: text, Chars: uint(len(runes))}}, nil
	}

	step := int(options.SizeChars - options.OverlapChars)
	size := int(options.SizeChars)
	chunks := make([]TextChunk, 0, len(runes)/step+1)
	for start := 0; start < len(runes); start += step {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			chunks = append(chunks, TextChunk{Index: uint(len(chunks) + 1), Content: part, Chars: uint(len([]rune(part)))})
		}
		if end == len(runes) {
			break
		}
	}
	return chunks, nil
}
