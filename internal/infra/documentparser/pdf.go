package documentparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

type pdfParser struct{}

func (pdfParser) Name() string    { return string(FormatPDF) }
func (pdfParser) Version() string { return "1" }

func (pdfParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatPDF)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF-")) {
		return nil, parseError(FormatPDF, fmt.Errorf("%w: PDF signature", ErrMalformedDocument))
	}
	if bytes.Contains(data, []byte("/Encrypt")) {
		return nil, parseError(FormatPDF, ErrEncryptedPDF)
	}
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, parseError(FormatPDF, fmt.Errorf("%w: PDF: %v", ErrMalformedDocument, err))
	}
	pageCount := reader.NumPage()
	if pageCount <= 0 {
		return nil, parseError(FormatPDF, fmt.Errorf("%w: PDF has no pages", ErrMalformedDocument))
	}
	if uint32(pageCount) > limits.MaxPages {
		return nil, parseError(FormatPDF, fmt.Errorf("%w: PDF pages", ErrLimitExceeded))
	}

	collector := newBlockCollector(limits)
	for pageNumber := 1; pageNumber <= pageCount; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := reader.Page(pageNumber).GetPlainText(nil)
		if err != nil {
			return nil, parseError(FormatPDF, fmt.Errorf("%w: PDF page %d: %v", ErrMalformedDocument, pageNumber, err))
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		page := uint32(pageNumber)
		if err := collector.add(value, nil, ContextLocatorV1{Kind: "pdf_page", Page: &page}); err != nil {
			return nil, parseError(FormatPDF, err)
		}
	}
	if len(collector.blocks) == 0 {
		return nil, parseError(FormatPDF, ErrUnsupportedScannedPDF)
	}
	return collector.blocks, nil
}
