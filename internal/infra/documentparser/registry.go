package documentparser

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strings"
)

type Registry struct {
	parsers    map[Format]Parser
	extensions map[string]Format
	mimeTypes  map[string]Format
}

func NewRegistry() *Registry {
	parsers := []struct {
		format Format
		parser Parser
	}{
		{FormatTXT, txtParser{}},
		{FormatMarkdown, markdownParser{}},
		{FormatPDF, pdfParser{}},
		{FormatDOCX, docxParser{}},
		{FormatCSV, csvParser{}},
		{FormatXLSX, xlsxParser{}},
	}
	registry := &Registry{
		parsers: make(map[Format]Parser, len(parsers)),
		extensions: map[string]Format{
			".txt": FormatTXT, ".md": FormatMarkdown, ".markdown": FormatMarkdown,
			".pdf": FormatPDF, ".docx": FormatDOCX, ".csv": FormatCSV, ".xlsx": FormatXLSX,
		},
		mimeTypes: map[string]Format{
			"text/plain": FormatTXT, "text/markdown": FormatMarkdown, "text/x-markdown": FormatMarkdown,
			"application/pdf": FormatPDF, MIMETypeDOCX: FormatDOCX,
			"text/csv": FormatCSV, "application/csv": FormatCSV, MIMETypeXLSX: FormatXLSX,
		},
	}
	for _, entry := range parsers {
		registry.parsers[entry.format] = entry.parser
	}
	return registry
}

func (registry *Registry) Formats() []Format {
	formats := make([]Format, 0, len(registry.parsers))
	for format := range registry.parsers {
		formats = append(formats, format)
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i] < formats[j] })
	return formats
}

func (registry *Registry) Resolve(filename, mimeType string) (Parser, error) {
	if registry == nil {
		return nil, parseError("", fmt.Errorf("%w: nil registry", ErrUnsupportedFormat))
	}
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	extensionFormat, extensionKnown := registry.extensions[extension]
	normalizedMIME, err := normalizeMIME(mimeType)
	if err != nil {
		return nil, parseError("", fmt.Errorf("%w: %v", ErrUnsupportedFormat, err))
	}
	mimeFormat, mimeKnown := registry.mimeTypes[normalizedMIME]

	if extensionKnown && mimeKnown && extensionFormat != mimeFormat {
		return nil, parseError(extensionFormat, ErrFormatMismatch)
	}
	if !extensionKnown {
		return nil, parseError("", fmt.Errorf("%w: extension %q", ErrUnsupportedFormat, extension))
	}
	if !mimeKnown && normalizedMIME != "application/octet-stream" {
		return nil, parseError(extensionFormat, fmt.Errorf("%w: MIME type %q", ErrUnsupportedFormat, normalizedMIME))
	}
	return registry.parsers[extensionFormat], nil
}

func (registry *Registry) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parser, err := registry.Resolve(source.Filename, source.MIMEType)
	if err != nil {
		return nil, err
	}
	return parser.Parse(ctx, source, limits)
}

func normalizeMIME(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return strings.ToLower(mediaType), nil
}
