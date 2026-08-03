package documentparser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

type docxParser struct{}

func (docxParser) Name() string    { return string(FormatDOCX) }
func (docxParser) Version() string { return "1" }

func (docxParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatDOCX)
	if err != nil {
		return nil, err
	}
	archive, err := inspectZIP(ctx, data, limits, FormatDOCX)
	if err != nil {
		return nil, err
	}

	var document *zip.File
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.ToLower(strings.ReplaceAll(file.Name, "\\", "/"))
		if strings.HasSuffix(name, "vbaproject.bin") || strings.Contains(name, "/activex/") {
			return nil, parseError(FormatDOCX, fmt.Errorf("%w: active content", ErrUnsafeDocument))
		}
		switch {
		case name == "word/document.xml":
			document = file
		case strings.HasSuffix(name, ".rels"):
			content, err := readZIPEntry(ctx, file, limits.MaxExpandedBytes)
			if err != nil {
				return nil, parseError(FormatDOCX, err)
			}
			if err := rejectExternalRelationships(content); err != nil {
				return nil, parseError(FormatDOCX, err)
			}
		case name == "[content_types].xml":
			content, err := readZIPEntry(ctx, file, limits.MaxExpandedBytes)
			if err != nil {
				return nil, parseError(FormatDOCX, err)
			}
			if bytes.Contains(bytes.ToLower(content), []byte("macroenabled")) {
				return nil, parseError(FormatDOCX, fmt.Errorf("%w: macro-enabled content type", ErrUnsafeDocument))
			}
		}
	}
	if document == nil {
		return nil, parseError(FormatDOCX, fmt.Errorf("%w: word/document.xml missing", ErrMalformedDocument))
	}
	content, err := readZIPEntry(ctx, document, limits.MaxExpandedBytes)
	if err != nil {
		return nil, parseError(FormatDOCX, err)
	}
	return parseDocumentXML(ctx, content, limits)
}

func parseDocumentXML(ctx context.Context, content []byte, limits Limits) ([]Block, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = true
	collector := newBlockCollector(limits)
	headings := make([]string, 0, 9)
	var paragraphText strings.Builder
	var paragraphStyle string
	var paragraphNumber uint32
	insideParagraph := false
	insideText := false

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, parseError(FormatDOCX, fmt.Errorf("%w: document XML: %v", ErrMalformedDocument, err))
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "p":
				insideParagraph = true
				paragraphNumber++
				if paragraphNumber > limits.MaxParagraphs {
					return nil, parseError(FormatDOCX, fmt.Errorf("%w: DOCX paragraphs", ErrLimitExceeded))
				}
				paragraphText.Reset()
				paragraphStyle = ""
			case "pStyle":
				if insideParagraph {
					paragraphStyle = attributeValue(typed.Attr, "val")
				}
			case "t":
				insideText = insideParagraph
			case "tab", "br":
				if !insideParagraph {
					continue
				}
				if typed.Name.Local == "tab" {
					paragraphText.WriteByte('\t')
				} else if typed.Name.Local == "br" {
					paragraphText.WriteByte('\n')
				}
			}
		case xml.CharData:
			if insideText {
				paragraphText.Write([]byte(typed))
			}
		case xml.EndElement:
			if typed.Name.Local == "t" {
				insideText = false
				continue
			}
			if typed.Name.Local != "p" || !insideParagraph {
				continue
			}
			insideParagraph = false
			value := strings.TrimSpace(paragraphText.String())
			if level := docxHeadingLevel(paragraphStyle); level > 0 && value != "" {
				if len(headings) >= level {
					headings = headings[:level-1]
				}
				for len(headings) < level-1 {
					headings = append(headings, "")
				}
				headings = append(headings, value)
			}
			paragraph := paragraphNumber
			if err := collector.add(value, compactHeadingPath(headings), ContextLocatorV1{Kind: "docx_paragraph", Paragraph: &paragraph}); err != nil {
				return nil, parseError(FormatDOCX, err)
			}
		}
	}
	return collector.result(FormatDOCX)
}

func docxHeadingLevel(style string) int {
	style = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(style), " ", ""))
	for _, prefix := range []string{"heading", "title"} {
		if !strings.HasPrefix(style, prefix) {
			continue
		}
		if prefix == "title" {
			return 1
		}
		level, err := strconv.Atoi(strings.TrimPrefix(style, prefix))
		if err == nil && level >= 1 && level <= 9 {
			return level
		}
	}
	return 0
}

func attributeValue(attributes []xml.Attr, local string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == local {
			return attribute.Value
		}
	}
	return ""
}

func rejectExternalRelationships(content []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = true
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: relationships XML: %v", ErrMalformedDocument, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		if strings.EqualFold(attributeValue(start.Attr, "TargetMode"), "External") {
			return fmt.Errorf("%w: external relationship", ErrUnsafeDocument)
		}
	}
}

func inspectZIP(ctx context.Context, data []byte, limits Limits, format Format) (*zip.Reader, error) {
	if len(data) < 4 || !bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) {
		return nil, parseError(format, fmt.Errorf("%w: ZIP signature", ErrMalformedDocument))
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, parseError(format, fmt.Errorf("%w: ZIP: %v", ErrMalformedDocument, err))
	}
	if len(archive.File) > int(limits.MaxZipEntries) {
		return nil, parseError(format, fmt.Errorf("%w: ZIP entries", ErrLimitExceeded))
	}
	var expanded uint64
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		if strings.HasPrefix(clean, "../") || clean == ".." || path.IsAbs(clean) {
			return nil, parseError(format, fmt.Errorf("%w: unsafe ZIP path", ErrUnsafeDocument))
		}
		if ^uint64(0)-expanded < file.UncompressedSize64 {
			return nil, parseError(format, fmt.Errorf("%w: ZIP expanded bytes overflow", ErrLimitExceeded))
		}
		expanded += file.UncompressedSize64
		if expanded > uint64(limits.MaxExpandedBytes) {
			return nil, parseError(format, fmt.Errorf("%w: ZIP expanded bytes", ErrLimitExceeded))
		}
	}
	compressed := uint64(len(data))
	ratio := uint64(limits.MaxExpansionRatio)
	if compressed == 0 || expanded/compressed > ratio || (expanded/compressed == ratio && expanded%compressed != 0) {
		return nil, parseError(format, fmt.Errorf("%w: ZIP expansion ratio", ErrLimitExceeded))
	}
	return archive, nil
}

func readZIPEntry(ctx context.Context, file *zip.File, maxBytes int64) ([]byte, error) {
	if file.UncompressedSize64 > uint64(maxBytes) {
		return nil, fmt.Errorf("%w: ZIP entry bytes", ErrLimitExceeded)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open ZIP entry: %v", ErrMalformedDocument, err)
	}
	defer func() { _ = reader.Close() }()
	return readBounded(ctx, reader, maxBytes)
}

func readBounded(ctx context.Context, reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	var output bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, err := limited.Read(buffer)
		if read > 0 {
			_, _ = output.Write(buffer[:read])
			if int64(output.Len()) > maxBytes {
				return nil, fmt.Errorf("%w: expanded bytes", ErrLimitExceeded)
			}
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: read expanded content: %v", ErrMalformedDocument, err)
		}
		if read == 0 {
			return nil, fmt.Errorf("%w: expanded reader made no progress", ErrMalformedDocument)
		}
	}
}
