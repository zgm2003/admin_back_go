package exporttask

import (
	"fmt"

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
