package documentparser

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	ContextLocatorSchemaV1 = "context_locator_v1"
	MIMETypeDOCX           = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MIMETypeXLSX           = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var (
	ErrDocumentParseFailed   = errors.New("ai.context.document_parse_failed")
	ErrUnsupportedFormat     = errors.New("unsupported document format")
	ErrFormatMismatch        = errors.New("document MIME type and extension disagree")
	ErrInvalidEncoding       = errors.New("invalid document encoding")
	ErrBinaryDisguise        = errors.New("binary content disguised as text")
	ErrMalformedDocument     = errors.New("malformed document")
	ErrUnsupportedScannedPDF = errors.New("scanned PDF has no supported text layer")
	ErrEncryptedPDF          = errors.New("encrypted PDF is not supported")
	ErrUnsafeDocument        = errors.New("unsafe document package")
	ErrLimitExceeded         = errors.New("document parsing limit exceeded")
)

type Format string

const (
	FormatTXT      Format = "txt"
	FormatMarkdown Format = "markdown"
	FormatPDF      Format = "pdf"
	FormatDOCX     Format = "docx"
	FormatCSV      Format = "csv"
	FormatXLSX     Format = "xlsx"
)

type Source struct {
	Filename string
	MIMEType string
	Size     int64
	Reader   io.Reader
}

type ContextLocatorV1 struct {
	Schema      string   `json:"schema"`
	Kind        string   `json:"kind"`
	Page        *uint32  `json:"page,omitempty"`
	Paragraph   *uint32  `json:"paragraph,omitempty"`
	LineStart   *uint32  `json:"line_start,omitempty"`
	LineEnd     *uint32  `json:"line_end,omitempty"`
	RowStart    *uint32  `json:"row_start,omitempty"`
	RowEnd      *uint32  `json:"row_end,omitempty"`
	Sheet       *string  `json:"sheet,omitempty"`
	CellStart   *string  `json:"cell_start,omitempty"`
	CellEnd     *string  `json:"cell_end,omitempty"`
	HeadingPath []string `json:"heading_path,omitempty"`
}

type Block struct {
	Ordinal     uint32
	Text        string
	HeadingPath []string
	Locator     ContextLocatorV1
}

type Parser interface {
	Name() string
	Version() string
	Parse(context.Context, Source, Limits) ([]Block, error)
}

type ParseError struct {
	Format Format
	Cause  error
}

func (err *ParseError) Error() string {
	if err.Format == "" {
		return fmt.Sprintf("%s: %v", ErrDocumentParseFailed, err.Cause)
	}
	return fmt.Sprintf("%s: %s: %v", ErrDocumentParseFailed, err.Format, err.Cause)
}

func (err *ParseError) Unwrap() []error {
	return []error{ErrDocumentParseFailed, err.Cause}
}

func parseError(format Format, cause error) error {
	if cause == nil {
		cause = ErrMalformedDocument
	}
	var existing *ParseError
	if errors.As(cause, &existing) {
		return cause
	}
	return &ParseError{Format: format, Cause: cause}
}
