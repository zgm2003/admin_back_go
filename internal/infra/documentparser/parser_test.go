package documentparser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/xuri/excelize/v2"
)

func TestRegistryContainsExactlySixFormats(t *testing.T) {
	registry := NewRegistry()
	want := []Format{FormatCSV, FormatDOCX, FormatMarkdown, FormatPDF, FormatTXT, FormatXLSX}
	if diff := cmp.Diff(want, registry.Formats()); diff != "" {
		t.Fatalf("formats mismatch (-want +got):\n%s", diff)
	}

	tests := []struct {
		filename string
		mimeType string
		want     Format
	}{
		{"notes.txt", "text/plain; charset=utf-8", FormatTXT},
		{"notes.md", "text/markdown", FormatMarkdown},
		{"report.pdf", "application/pdf", FormatPDF},
		{"report.docx", MIMETypeDOCX, FormatDOCX},
		{"rows.csv", "text/csv", FormatCSV},
		{"book.xlsx", MIMETypeXLSX, FormatXLSX},
	}
	for _, tt := range tests {
		parser, err := registry.Resolve(tt.filename, tt.mimeType)
		if err != nil {
			t.Fatalf("Resolve(%q, %q): %v", tt.filename, tt.mimeType, err)
		}
		if parser.Name() != string(tt.want) {
			t.Fatalf("Resolve(%q, %q) parser=%q, want %q", tt.filename, tt.mimeType, parser.Name(), tt.want)
		}
	}
}

func TestRegistryRejectsFormatMismatch(t *testing.T) {
	for _, tt := range []struct {
		filename string
		mimeType string
		want     error
	}{
		{"disguised.txt", "application/pdf", ErrFormatMismatch},
		{"unknown.txt", "application/octet-stream", ErrUnsupportedFormat},
		{"unknown.bin", "text/plain", ErrUnsupportedFormat},
	} {
		_, err := NewRegistry().Resolve(tt.filename, tt.mimeType)
		if !errors.Is(err, tt.want) || !errors.Is(err, ErrDocumentParseFailed) {
			t.Fatalf("Resolve(%q, %q) error=%v, want %v parse failure", tt.filename, tt.mimeType, err, tt.want)
		}
	}
}

func TestTXTParsesUTF8AndUTF16LEWithLineLocators(t *testing.T) {
	registry := NewRegistry()
	want := []Block{
		{Ordinal: 0, Text: "first", Locator: lineLocator("txt_line", 1, 1)},
		{Ordinal: 1, Text: "second", Locator: lineLocator("txt_line", 3, 3)},
	}

	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"utf8", []byte("first\n\nsecond\n")},
		{"utf16le", utf16LE("first\n\nsecond\n")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := registry.Parse(context.Background(), source("notes.txt", "text/plain", tt.data), testLimits())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if diff := cmp.Diff(want, blocks); diff != "" {
				t.Fatalf("blocks mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTXTRejectsInvalidEncodingAndBinaryDisguise(t *testing.T) {
	registry := NewRegistry()
	for _, data := range [][]byte{{0xff, 0xfe, 0x00}, []byte("%PDF-1.7\nnot text"), {0x00, 0x01, 0x02, 0x03}} {
		_, err := registry.Parse(context.Background(), source("notes.txt", "text/plain", data), testLimits())
		if !errors.Is(err, ErrInvalidEncoding) && !errors.Is(err, ErrBinaryDisguise) {
			t.Fatalf("Parse(%x) error=%v, want encoding or disguise failure", data, err)
		}
	}
}

func TestMarkdownPreservesStructuralBlocksAndHeadingPaths(t *testing.T) {
	data := []byte("# Guide\n\nIntro.\n\n## Steps\n\n- one\n- two\n\n| A | B |\n|---|---|\n| x | y |\n\n```go\nfmt.Println(1)\n```\n")
	blocks, err := NewRegistry().Parse(context.Background(), source("guide.md", "text/markdown", data), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	wantText := []string{"Guide", "Intro.", "Steps", "one", "two", "A\tB\nx\ty", "fmt.Println(1)"}
	if diff := cmp.Diff(wantText, blockTexts(blocks)); diff != "" {
		t.Fatalf("texts mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Guide", "Steps"}, blocks[3].HeadingPath); diff != "" {
		t.Fatalf("list heading path mismatch (-want +got):\n%s", diff)
	}
	for i, block := range blocks {
		if block.Ordinal != uint32(i) || block.Locator.Kind != "markdown_block" {
			t.Fatalf("block[%d]=%+v, want deterministic markdown ordinal", i, block)
		}
	}
}

func TestCSVParsesRowsAndRejectsMalformedInput(t *testing.T) {
	blocks, err := NewRegistry().Parse(context.Background(), source("rows.csv", "text/csv", []byte("name,note\nAda,hello\nLinus,world\n")), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if diff := cmp.Diff([]string{"name\tnote", "Ada\thello", "Linus\tworld"}, blockTexts(blocks)); diff != "" {
		t.Fatalf("texts mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(rowLocator("csv_rows", 2, 2), blocks[1].Locator); diff != "" {
		t.Fatalf("locator mismatch (-want +got):\n%s", diff)
	}

	_, err = NewRegistry().Parse(context.Background(), source("bad.csv", "text/csv", []byte("a,b\n\"unterminated")), testLimits())
	if !errors.Is(err, ErrMalformedDocument) {
		t.Fatalf("malformed CSV error=%v, want %v", err, ErrMalformedDocument)
	}
}

func TestDOCXParsesParagraphsAndRejectsUnsafePackages(t *testing.T) {
	registry := NewRegistry()
	safe := makeDOCX(t, map[string]string{
		"[Content_Types].xml": contentTypesXML,
		"word/document.xml":   `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Title</w:t></w:r></w:p><w:p><w:r><w:t>Hello </w:t></w:r><w:r><w:t>DOCX</w:t></w:r></w:p></w:body></w:document>`,
	})
	blocks, err := registry.Parse(context.Background(), source("safe.docx", MIMETypeDOCX, safe), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if diff := cmp.Diff([]string{"Title", "Hello DOCX"}, blockTexts(blocks)); diff != "" {
		t.Fatalf("texts mismatch (-want +got):\n%s", diff)
	}
	if blocks[1].Locator.Paragraph == nil || *blocks[1].Locator.Paragraph != 2 {
		t.Fatalf("paragraph locator=%+v, want paragraph 2", blocks[1].Locator)
	}
	if diff := cmp.Diff([]string{"Title"}, blocks[1].HeadingPath); diff != "" {
		t.Fatalf("heading mismatch (-want +got):\n%s", diff)
	}

	unsafe := []struct {
		name  string
		files map[string]string
		want  error
	}{
		{"macro", map[string]string{"[Content_Types].xml": contentTypesXML, "word/document.xml": simpleDocumentXML, "word/vbaProject.bin": "macro"}, ErrUnsafeDocument},
		{"external", map[string]string{"[Content_Types].xml": contentTypesXML, "word/document.xml": simpleDocumentXML, "word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" TargetMode="External" Target="https://example.com/x"/></Relationships>`}, ErrUnsafeDocument},
	}
	for _, tt := range unsafe {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.Parse(context.Background(), source("unsafe.docx", MIMETypeDOCX, makeDOCX(t, tt.files)), testLimits())
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse error=%v, want %v", err, tt.want)
			}
		})
	}
}

func TestDOCXIgnoresFormattingWhitespaceOutsideTextNodes(t *testing.T) {
	document := `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Normal"/></w:pPr>
      <w:r><w:rPr><w:b/></w:rPr><w:t>Hello</w:t></w:r>
      <w:r><w:t xml:space="preserve"> world</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`
	data := makeDOCX(t, map[string]string{"[Content_Types].xml": contentTypesXML, "word/document.xml": document})
	blocks, err := NewRegistry().Parse(context.Background(), source("formatted.docx", MIMETypeDOCX, data), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := blockTexts(blocks); !cmp.Equal(got, []string{"Hello world"}) {
		t.Fatalf("texts=%q, want exact DOCX text", got)
	}
}

func TestDOCXRejectsExpansionLimit(t *testing.T) {
	data := makeDOCX(t, map[string]string{"[Content_Types].xml": contentTypesXML, "word/document.xml": simpleDocumentXML + strings.Repeat(" ", 32<<10)})
	limits := testLimits()
	limits.MaxExpandedBytes = 1024
	_, err := NewRegistry().Parse(context.Background(), source("large.docx", MIMETypeDOCX, data), limits)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Parse error=%v, want %v", err, ErrLimitExceeded)
	}
}

func TestXLSXParsesSheetsAndCellRanges(t *testing.T) {
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	file.SetSheetName("Sheet1", "People")
	if err := file.SetSheetRow("People", "A1", &[]any{"name", "role"}); err != nil {
		t.Fatal(err)
	}
	if err := file.SetSheetRow("People", "A2", &[]any{"Ada", "engineer"}); err != nil {
		t.Fatal(err)
	}
	data, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	blocks, err := NewRegistry().Parse(context.Background(), source("people.xlsx", MIMETypeXLSX, data.Bytes()), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if diff := cmp.Diff([]string{"name\trole", "Ada\tengineer"}, blockTexts(blocks)); diff != "" {
		t.Fatalf("texts mismatch (-want +got):\n%s", diff)
	}
	want := cellLocator("xlsx_range", "People", "A2", "B2")
	if diff := cmp.Diff(want, blocks[1].Locator); diff != "" {
		t.Fatalf("locator mismatch (-want +got):\n%s", diff)
	}

	_, err = NewRegistry().Parse(context.Background(), source("bad.xlsx", MIMETypeXLSX, []byte("not a workbook")), testLimits())
	if !errors.Is(err, ErrMalformedDocument) {
		t.Fatalf("corrupt XLSX error=%v, want %v", err, ErrMalformedDocument)
	}
}

func TestPDFParsesTextLayerAndRejectsScannedOrEncrypted(t *testing.T) {
	registry := NewRegistry()
	blocks, err := registry.Parse(context.Background(), source("text.pdf", "application/pdf", minimalPDF("Hello PDF")), testLimits())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(blocks) != 1 || !strings.Contains(blocks[0].Text, "Hello PDF") || blocks[0].Locator.Page == nil || *blocks[0].Locator.Page != 1 {
		t.Fatalf("blocks=%+v, want page 1 text", blocks)
	}

	_, err = registry.Parse(context.Background(), source("scan.pdf", "application/pdf", minimalPDF("")), testLimits())
	if !errors.Is(err, ErrUnsupportedScannedPDF) {
		t.Fatalf("scanned PDF error=%v, want %v", err, ErrUnsupportedScannedPDF)
	}
	_, err = registry.Parse(context.Background(), source("secret.pdf", "application/pdf", []byte("%PDF-1.7\n1 0 obj << /Encrypt 2 0 R >> endobj\n%%EOF")), testLimits())
	if !errors.Is(err, ErrEncryptedPDF) {
		t.Fatalf("encrypted PDF error=%v, want %v", err, ErrEncryptedPDF)
	}
}

func TestPDFEnforcesPageLimitBeforeExtractingPages(t *testing.T) {
	data := bytes.Replace(minimalPDF("Hello"), []byte("/Count 1"), []byte("/Count 2"), 1)
	limits := testLimits()
	limits.MaxPages = 1
	_, err := NewRegistry().Parse(context.Background(), source("two.pdf", "application/pdf", data), limits)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Parse error=%v, want %v", err, ErrLimitExceeded)
	}
}

func TestXLSXEnforcesSheetAndCellLimits(t *testing.T) {
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if _, err := file.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetSheetRow("Sheet1", "A1", &[]any{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	data, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	sheetLimits := testLimits()
	sheetLimits.MaxSheets = 1
	_, err = NewRegistry().Parse(context.Background(), source("book.xlsx", MIMETypeXLSX, data.Bytes()), sheetLimits)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("sheet limit error=%v, want %v", err, ErrLimitExceeded)
	}
	cellLimits := testLimits()
	cellLimits.MaxCells = 1
	_, err = NewRegistry().Parse(context.Background(), source("book.xlsx", MIMETypeXLSX, data.Bytes()), cellLimits)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("cell limit error=%v, want %v", err, ErrLimitExceeded)
	}
}

func TestParsersEnforceSourceTextAndStructuralLimits(t *testing.T) {
	registry := NewRegistry()
	tests := []struct {
		name   string
		source Source
		limits func() Limits
	}{
		{"source", source("a.txt", "text/plain", []byte("abcdef")), func() Limits { l := testLimits(); l.MaxSourceBytes = 3; return l }},
		{"text", source("a.txt", "text/plain", []byte("abcdef")), func() Limits { l := testLimits(); l.MaxTextBytes = 3; return l }},
		{"blocks", source("a.txt", "text/plain", []byte("a\nb\n")), func() Limits { l := testLimits(); l.MaxBlocks = 1; return l }},
		{"rows", source("a.csv", "text/csv", []byte("a\nb\n")), func() Limits { l := testLimits(); l.MaxRows = 1; return l }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.Parse(context.Background(), tt.source, tt.limits())
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("Parse error=%v, want %v", err, ErrLimitExceeded)
			}
		})
	}
}

func TestParserHonorsCancelledContextBeforeReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &panicReader{}
	_, err := NewRegistry().Parse(ctx, Source{Filename: "a.txt", MIMEType: "text/plain", Size: 1, Reader: reader}, testLimits())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse error=%v, want context cancellation", err)
	}
}

func TestParserHonorsCancellationWhileReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	data := bytes.Repeat([]byte("x"), 64<<10)
	reader := &cancelAfterReader{reader: bytes.NewReader(data), cancel: cancel}
	_, err := NewRegistry().Parse(ctx, Source{Filename: "a.txt", MIMEType: "text/plain", Size: int64(len(data)), Reader: reader}, testLimits())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse error=%v, want context cancellation", err)
	}
}

type panicReader struct{}

func (*panicReader) Read([]byte) (int, error) { panic("reader must not be called after cancellation") }

type cancelAfterReader struct {
	reader io.Reader
	cancel context.CancelFunc
	reads  int
}

func (reader *cancelAfterReader) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	reader.reads++
	if reader.reads == 1 {
		reader.cancel()
	}
	return read, err
}

func testLimits() Limits {
	return Limits{
		MaxSourceBytes:    2 << 20,
		MaxExpandedBytes:  4 << 20,
		MaxTextBytes:      1 << 20,
		MaxBlocks:         1000,
		MaxPages:          20,
		MaxParagraphs:     1000,
		MaxRows:           1000,
		MaxColumns:        100,
		MaxSheets:         20,
		MaxCells:          10_000,
		MaxZipEntries:     100,
		MaxExpansionRatio: 100,
	}
}

func source(filename, mimeType string, data []byte) Source {
	return Source{Filename: filename, MIMEType: mimeType, Size: int64(len(data)), Reader: bytes.NewReader(data)}
}

func blockTexts(blocks []Block) []string {
	texts := make([]string, len(blocks))
	for i := range blocks {
		texts[i] = blocks[i].Text
	}
	return texts
}

func lineLocator(kind string, start, end uint32) ContextLocatorV1 {
	return ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: kind, LineStart: &start, LineEnd: &end}
}

func rowLocator(kind string, start, end uint32) ContextLocatorV1 {
	return ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: kind, RowStart: &start, RowEnd: &end}
}

func cellLocator(kind, sheet, start, end string) ContextLocatorV1 {
	return ContextLocatorV1{Schema: ContextLocatorSchemaV1, Kind: kind, Sheet: &sheet, CellStart: &start, CellEnd: &end}
}

func utf16LE(value string) []byte {
	data := []byte{0xff, 0xfe}
	for _, r := range value {
		if r > 0xffff {
			panic("test helper only supports BMP runes")
		}
		var encoded [2]byte
		binary.LittleEndian.PutUint16(encoded[:], uint16(r))
		data = append(data, encoded[:]...)
	}
	return data
}

const contentTypesXML = `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`
const simpleDocumentXML = `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello</w:t></w:r></w:p></w:body></w:document>`

func makeDOCX(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func minimalPDF(text string) []byte {
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len("BT /F1 12 Tf 40 250 Td ("+text+") Tj ET"), "BT /F1 12 Tf 40 250 Td ("+text+") Tj ET"),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`,
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}
