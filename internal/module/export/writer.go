package exporttask

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

type Column struct {
	Key   string
	Title string
}

type FileData struct {
	Headers []Column
	Rows    []map[string]string
	Prefix  string
}

type XLSXWriter struct{}

func (XLSXWriter) Write(data FileData) ([]byte, error) {
	if len(data.Headers) == 0 {
		return nil, fmt.Errorf("xlsx writer: headers are required")
	}
	file := excelize.NewFile()
	defer file.Close()
	const sheet = "Sheet1"

	for col, header := range data.Headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return nil, fmt.Errorf("xlsx writer: header cell: %w", err)
		}
		if err := file.SetCellStr(sheet, cell, header.Title); err != nil {
			return nil, fmt.Errorf("xlsx writer: set header: %w", err)
		}
	}
	for rowIndex, row := range data.Rows {
		for col, header := range data.Headers {
			cell, err := excelize.CoordinatesToCellName(col+1, rowIndex+2)
			if err != nil {
				return nil, fmt.Errorf("xlsx writer: row cell: %w", err)
			}
			if err := file.SetCellStr(sheet, cell, row[header.Key]); err != nil {
				return nil, fmt.Errorf("xlsx writer: set row: %w", err)
			}
		}
	}
	buf, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("xlsx writer: write buffer: %w", err)
	}
	return buf.Bytes(), nil
}

type CSVWriter struct {
	newWriter func(io.Writer) *csv.Writer
}

func (writer CSVWriter) Write(data FileData) ([]byte, error) {
	if err := validateHeaders(data.Headers); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	buffer.Write([]byte{0xef, 0xbb, 0xbf})
	newWriter := writer.newWriter
	if newWriter == nil {
		newWriter = csv.NewWriter
	}
	csvWriter := newWriter(&buffer)
	csvWriter.UseCRLF = true

	header := make([]string, len(data.Headers))
	for index, column := range data.Headers {
		header[index] = neutralizeFormula(column.Title)
	}
	if err := csvWriter.Write(header); err != nil {
		return nil, fmt.Errorf("csv writer: write headers: %w", err)
	}
	for _, row := range data.Rows {
		record := make([]string, len(data.Headers))
		for index, column := range data.Headers {
			record[index] = neutralizeFormula(row[column.Key])
		}
		if err := csvWriter.Write(record); err != nil {
			return nil, fmt.Errorf("csv writer: write row: %w", err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, fmt.Errorf("csv writer: flush: %w", err)
	}
	return buffer.Bytes(), nil
}

func validateHeaders(headers []Column) error {
	if len(headers) == 0 {
		return fmt.Errorf("csv writer: headers are required")
	}
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" || strings.TrimSpace(header.Title) == "" {
			return fmt.Errorf("csv writer: header key and title are required")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("csv writer: duplicate header key")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func neutralizeFormula(value string) string {
	for _, character := range value {
		if unicode.IsSpace(character) {
			continue
		}
		switch character {
		case '=', '+', '-', '@':
			return "'" + value
		default:
			return value
		}
	}
	return value
}
