package documentparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	markdownast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

type markdownParser struct{}

func (markdownParser) Name() string    { return string(FormatMarkdown) }
func (markdownParser) Version() string { return "1" }

func (markdownParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatMarkdown)
	if err != nil {
		return nil, err
	}
	if err := rejectTextDisguise(data); err != nil {
		return nil, parseError(FormatMarkdown, err)
	}
	if !utf8.Valid(data) {
		return nil, parseError(FormatMarkdown, ErrInvalidEncoding)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})

	parser := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()
	document := parser.Parse(text.NewReader(data))
	collector := newBlockCollector(limits)
	headings := make([]string, 0, 6)
	var structuralBlocks uint32

	var visit func(markdownast.Node) error
	visit = func(node markdownast.Node) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch typed := node.(type) {
		case *markdownast.Heading:
			value := inlineNodeText(typed, data)
			level := typed.Level
			if len(headings) >= level {
				headings = headings[:level-1]
			}
			for len(headings) < level-1 {
				headings = append(headings, "")
			}
			headings = append(headings, value)
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(value, compactHeadingPath(headings), markdownLocator(data, typed))
		case *markdownast.Paragraph:
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(inlineNodeText(typed, data), compactHeadingPath(headings), markdownLocator(data, typed))
		case *markdownast.TextBlock:
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(inlineNodeText(typed, data), compactHeadingPath(headings), markdownLocator(data, typed))
		case *markdownast.FencedCodeBlock:
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(blockLinesText(typed, data), compactHeadingPath(headings), markdownLocator(data, typed))
		case *markdownast.CodeBlock:
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(blockLinesText(typed, data), compactHeadingPath(headings), markdownLocator(data, typed))
		case *extensionast.Table:
			structuralBlocks++
			if structuralBlocks > limits.MaxParagraphs {
				return fmt.Errorf("%w: Markdown structural blocks", ErrLimitExceeded)
			}
			return collector.add(markdownTableText(typed, data), compactHeadingPath(headings), markdownLocator(data, typed))
		}
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(document); err != nil {
		return nil, parseError(FormatMarkdown, err)
	}
	return collector.result(FormatMarkdown)
}

func inlineNodeText(node markdownast.Node, source []byte) string {
	var output strings.Builder
	_ = markdownast.Walk(node, func(current markdownast.Node, entering bool) (markdownast.WalkStatus, error) {
		if !entering {
			return markdownast.WalkContinue, nil
		}
		switch typed := current.(type) {
		case *markdownast.Text:
			output.Write(typed.Segment.Value(source))
			if typed.SoftLineBreak() || typed.HardLineBreak() {
				output.WriteByte('\n')
			}
		case *markdownast.String:
			output.Write(typed.Value)
		}
		return markdownast.WalkContinue, nil
	})
	return strings.TrimSpace(output.String())
}

func blockLinesText(node markdownast.Node, source []byte) string {
	var output bytes.Buffer
	lines := node.Lines()
	for index := 0; index < lines.Len(); index++ {
		segment := lines.At(index)
		output.Write(segment.Value(source))
	}
	return strings.TrimSpace(output.String())
}

func markdownTableText(table *extensionast.Table, source []byte) string {
	rows := make([]string, 0)
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		cells := make([]string, 0)
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if _, ok := cell.(*extensionast.TableCell); ok {
				cells = append(cells, inlineNodeText(cell, source))
			}
		}
		if len(cells) > 0 {
			rows = append(rows, strings.Join(cells, "\t"))
		}
	}
	return strings.Join(rows, "\n")
}

func markdownLocator(source []byte, node markdownast.Node) ContextLocatorV1 {
	start, stop := nodeOffsets(node)
	lineStart := lineAtOffset(source, start)
	lineEnd := lineAtOffset(source, stop)
	return ContextLocatorV1{Kind: "markdown_block", LineStart: &lineStart, LineEnd: &lineEnd}
}

func nodeOffsets(node markdownast.Node) (int, int) {
	lines := node.Lines()
	if lines != nil && lines.Len() > 0 {
		return lines.At(0).Start, lines.At(lines.Len() - 1).Stop
	}
	start, stop := -1, -1
	_ = markdownast.Walk(node, func(current markdownast.Node, entering bool) (markdownast.WalkStatus, error) {
		if !entering {
			return markdownast.WalkContinue, nil
		}
		if textNode, ok := current.(*markdownast.Text); ok {
			if start < 0 || textNode.Segment.Start < start {
				start = textNode.Segment.Start
			}
			if textNode.Segment.Stop > stop {
				stop = textNode.Segment.Stop
			}
		}
		return markdownast.WalkContinue, nil
	})
	if start < 0 {
		return 0, 0
	}
	return start, stop
}

func lineAtOffset(source []byte, offset int) uint32 {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	return uint32(bytes.Count(source[:offset], []byte{'\n'}) + 1)
}

func compactHeadingPath(headings []string) []string {
	result := make([]string, 0, len(headings))
	for _, heading := range headings {
		if heading != "" {
			result = append(result, heading)
		}
	}
	return result
}
