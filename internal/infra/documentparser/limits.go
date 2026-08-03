package documentparser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type Limits struct {
	MaxSourceBytes    int64
	MaxExpandedBytes  int64
	MaxTextBytes      int64
	MaxBlocks         uint32
	MaxPages          uint32
	MaxParagraphs     uint32
	MaxRows           uint32
	MaxColumns        uint32
	MaxSheets         uint32
	MaxCells          uint64
	MaxZipEntries     uint32
	MaxExpansionRatio uint32
}

func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes:    64 << 20,
		MaxExpandedBytes:  256 << 20,
		MaxTextBytes:      16 << 20,
		MaxBlocks:         100_000,
		MaxPages:          2_000,
		MaxParagraphs:     100_000,
		MaxRows:           1_000_000,
		MaxColumns:        16_384,
		MaxSheets:         1_000,
		MaxCells:          5_000_000,
		MaxZipEntries:     20_000,
		MaxExpansionRatio: 100,
	}
}

func (limits Limits) Validate() error {
	if limits.MaxSourceBytes <= 0 || limits.MaxExpandedBytes <= 0 || limits.MaxTextBytes <= 0 ||
		limits.MaxBlocks == 0 || limits.MaxPages == 0 || limits.MaxParagraphs == 0 ||
		limits.MaxRows == 0 || limits.MaxColumns == 0 || limits.MaxSheets == 0 ||
		limits.MaxCells == 0 || limits.MaxZipEntries == 0 || limits.MaxExpansionRatio == 0 {
		return fmt.Errorf("%w: all limits must be positive", ErrLimitExceeded)
	}
	return nil
}

func readSource(ctx context.Context, source Source, limits Limits, format Format) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, parseError(format, err)
	}
	if strings.TrimSpace(source.Filename) == "" || strings.TrimSpace(source.MIMEType) == "" || source.Size < 0 || source.Reader == nil {
		return nil, parseError(format, fmt.Errorf("%w: invalid source facts", ErrMalformedDocument))
	}
	if source.Size > limits.MaxSourceBytes {
		return nil, parseError(format, fmt.Errorf("%w: source bytes", ErrLimitExceeded))
	}

	reader := &io.LimitedReader{R: source.Reader, N: limits.MaxSourceBytes + 1}
	var output bytes.Buffer
	if source.Size > 0 && source.Size <= limits.MaxSourceBytes {
		output.Grow(int(source.Size))
	}
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, err := reader.Read(buffer)
		if read > 0 {
			_, _ = output.Write(buffer[:read])
			if int64(output.Len()) > limits.MaxSourceBytes {
				return nil, parseError(format, fmt.Errorf("%w: source bytes", ErrLimitExceeded))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, parseError(format, fmt.Errorf("%w: read source: %v", ErrMalformedDocument, err))
		}
		if read == 0 {
			return nil, parseError(format, fmt.Errorf("%w: reader made no progress", ErrMalformedDocument))
		}
	}
	if source.Size != int64(output.Len()) {
		return nil, parseError(format, fmt.Errorf("%w: declared size %d differs from read size %d", ErrMalformedDocument, source.Size, output.Len()))
	}
	return output.Bytes(), nil
}

type blockCollector struct {
	limits    Limits
	blocks    []Block
	textBytes int64
}

func newBlockCollector(limits Limits) *blockCollector {
	capacity := int(limits.MaxBlocks)
	if capacity > 128 {
		capacity = 128
	}
	return &blockCollector{limits: limits, blocks: make([]Block, 0, capacity)}
}

func (collector *blockCollector) add(text string, headingPath []string, locator ContextLocatorV1) error {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("%w: extracted text is not UTF-8", ErrInvalidEncoding)
	}
	if len(collector.blocks) >= int(collector.limits.MaxBlocks) {
		return fmt.Errorf("%w: blocks", ErrLimitExceeded)
	}
	collector.textBytes += int64(len(text))
	if collector.textBytes > collector.limits.MaxTextBytes {
		return fmt.Errorf("%w: extracted text bytes", ErrLimitExceeded)
	}
	locator.Schema = ContextLocatorSchemaV1
	locator.HeadingPath = append([]string(nil), headingPath...)
	collector.blocks = append(collector.blocks, Block{
		Ordinal:     uint32(len(collector.blocks)),
		Text:        text,
		HeadingPath: append([]string(nil), headingPath...),
		Locator:     locator,
	})
	return nil
}

func (collector *blockCollector) result(format Format) ([]Block, error) {
	if len(collector.blocks) == 0 {
		return nil, parseError(format, fmt.Errorf("%w: no extractable text", ErrMalformedDocument))
	}
	return collector.blocks, nil
}

func rejectTextDisguise(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("%PDF-")) || bytes.HasPrefix(trimmed, []byte{'P', 'K', 3, 4}) || bytes.IndexByte(data, 0) >= 0 {
		return ErrBinaryDisguise
	}
	return nil
}

func rejectBinarySignature(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("%PDF-")) || bytes.HasPrefix(trimmed, []byte{'P', 'K', 3, 4}) {
		return ErrBinaryDisguise
	}
	return nil
}
