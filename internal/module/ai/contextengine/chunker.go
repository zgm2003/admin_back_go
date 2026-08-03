package contextengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
)

const ChunkerVersionV1 = "context_chunker_v1"

var (
	ErrChunkTokenUnitTooLarge = errors.New("context chunk token unit exceeds the embedding limit")
	ErrChunkFactsConflict     = errors.New("context chunk retry changed immutable facts")
)

type StructuralBlock struct {
	Ordinal     uint32
	Text        string
	HeadingPath []string
	Locator     ContextLocatorV1
}

type Chunk struct {
	Ordinal                       uint32
	SourceBlockOrdinal            uint32
	Text                          string
	IndexText                     string
	EmbeddingInputTokenUpperBound int64
	HeadingPath                   []string
	Locator                       ContextLocatorV1
	ContentSHA256                 [sha256.Size]byte
	ChunkFactsSHA256              [sha256.Size]byte
}

type Chunker struct {
	counter infraai.TokenCounter
	target  int64
	overlap int64
}

func NewChunker(counter infraai.TokenCounter, embeddingMaxInputTokens int64) (*Chunker, error) {
	if counter == nil || strings.TrimSpace(counter.ID()) == "" || embeddingMaxInputTokens <= 0 {
		return nil, errors.New("chunker requires a token counter and positive embedding limit")
	}
	target := min(int64(800), embeddingMaxInputTokens)
	return &Chunker{counter: counter, target: target, overlap: min(int64(80), target/10)}, nil
}

func (chunker *Chunker) Chunk(blocks []StructuralBlock) ([]Chunk, error) {
	if chunker == nil || chunker.counter == nil || chunker.target <= 0 {
		return nil, errors.New("chunker is not configured")
	}
	chunks := make([]Chunk, 0, len(blocks))
	for _, block := range blocks {
		if err := validateStructuralBlock(block); err != nil {
			return nil, err
		}
		prefix := indexTextPrefix(block.HeadingPath)
		windows, err := chunker.windows(prefix, block.Text)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", block.Ordinal, err)
		}
		for _, text := range windows {
			chunk, err := newChunk(uint32(len(chunks)), block, text, chunker.counter)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, chunk)
		}
	}
	return chunks, nil
}

func validateStructuralBlock(block StructuralBlock) error {
	if strings.TrimSpace(block.Text) == "" || !utf8.ValidString(block.Text) {
		return errors.New("structural block text must be non-empty UTF-8")
	}
	if err := block.Locator.Validate(); err != nil {
		return err
	}
	for _, heading := range block.HeadingPath {
		if strings.TrimSpace(heading) == "" || strings.TrimSpace(heading) != heading || !utf8.ValidString(heading) {
			return errors.New("structural block heading path is invalid")
		}
	}
	return nil
}

func (chunker *Chunker) windows(prefix, text string) ([]string, error) {
	prefixBound, err := chunker.counter.UpperBoundText(prefix)
	if err != nil {
		return nil, err
	}
	if prefixBound >= chunker.target {
		return nil, ErrChunkTokenUnitTooLarge
	}
	bound, err := chunker.counter.UpperBoundText(prefix + text)
	if err != nil {
		return nil, err
	}
	if bound <= chunker.target {
		return []string{text}, nil
	}
	runes := []rune(text)
	windows := make([]string, 0, 2)
	for start := 0; start < len(runes); {
		end, err := chunker.maxWindow(prefix, runes, start)
		if err != nil {
			return nil, err
		}
		windows = append(windows, string(runes[start:end]))
		if end == len(runes) {
			break
		}
		overlapStart := end
		for overlapStart > start && chunker.overlap > 0 {
			candidate := string(runes[overlapStart-1 : end])
			candidateBound, countErr := chunker.counter.UpperBoundText(candidate)
			if countErr != nil {
				return nil, countErr
			}
			if candidateBound > chunker.overlap {
				break
			}
			overlapStart--
		}
		if overlapStart == start {
			overlapStart = end
		}
		start = overlapStart
	}
	return windows, nil
}

func (chunker *Chunker) maxWindow(prefix string, runes []rune, start int) (int, error) {
	firstBound, err := chunker.counter.UpperBoundText(prefix + string(runes[start:start+1]))
	if err != nil {
		return 0, err
	}
	if firstBound > chunker.target {
		return 0, ErrChunkTokenUnitTooLarge
	}
	low, high := start+1, len(runes)+1
	for low+1 < high {
		middle := low + (high-low)/2
		bound, countErr := chunker.counter.UpperBoundText(prefix + string(runes[start:middle]))
		if countErr != nil {
			return 0, countErr
		}
		if bound <= chunker.target {
			low = middle
		} else {
			high = middle
		}
	}
	return low, nil
}

func newChunk(ordinal uint32, block StructuralBlock, text string, counter infraai.TokenCounter) (Chunk, error) {
	contentSHA := sha256.Sum256([]byte(text))
	locatorJSON, err := json.Marshal(block.Locator)
	if err != nil {
		return Chunk{}, fmt.Errorf("encode chunk locator: %w", err)
	}
	var facts bytes.Buffer
	facts.WriteString("context_chunk_facts_v1\x00")
	for _, heading := range block.HeadingPath {
		facts.WriteString(heading)
		facts.WriteByte(0)
	}
	facts.Write(contentSHA[:])
	facts.WriteByte(0)
	facts.Write(locatorJSON)
	indexText := indexTextPrefix(block.HeadingPath) + text
	bound, err := counter.UpperBoundText(indexText)
	if err != nil {
		return Chunk{}, err
	}
	return Chunk{
		Ordinal: ordinal, SourceBlockOrdinal: block.Ordinal, Text: text, IndexText: indexText,
		EmbeddingInputTokenUpperBound: bound,
		HeadingPath:                   append([]string(nil), block.HeadingPath...), Locator: block.Locator,
		ContentSHA256: contentSHA, ChunkFactsSHA256: sha256.Sum256(facts.Bytes()),
	}, nil
}

func indexTextPrefix(headingPath []string) string {
	if len(headingPath) == 0 {
		return ""
	}
	return strings.Join(headingPath, " > ") + "\n"
}

func ValidateChunkRetry(existing, retry []Chunk) error {
	if len(existing) != len(retry) {
		return ErrChunkFactsConflict
	}
	for index := range existing {
		if !reflect.DeepEqual(existing[index], retry[index]) {
			return fmt.Errorf("%w at ordinal %d", ErrChunkFactsConflict, index)
		}
	}
	return nil
}
