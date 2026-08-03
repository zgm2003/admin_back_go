package documentparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type xlsxParser struct{}

func (xlsxParser) Name() string    { return string(FormatXLSX) }
func (xlsxParser) Version() string { return "1" }

func (xlsxParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatXLSX)
	if err != nil {
		return nil, err
	}
	archive, err := inspectZIP(ctx, data, limits, FormatXLSX)
	if err != nil {
		return nil, err
	}
	for _, entry := range archive.File {
		name := strings.ToLower(strings.ReplaceAll(entry.Name, "\\", "/"))
		if strings.Contains(name, "vbaproject.bin") || strings.HasPrefix(name, "xl/externallinks/") {
			return nil, parseError(FormatXLSX, fmt.Errorf("%w: active or external workbook content", ErrUnsafeDocument))
		}
	}

	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook: %v", ErrMalformedDocument, err))
	}
	defer func() { _ = workbook.Close() }()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook has no sheets", ErrMalformedDocument))
	}
	if uint32(len(sheets)) > limits.MaxSheets {
		return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook sheets", ErrLimitExceeded))
	}

	collector := newBlockCollector(limits)
	var totalRows uint32
	var totalCells uint64
	for _, sheet := range sheets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := workbook.Rows(sheet)
		if err != nil {
			return nil, parseError(FormatXLSX, fmt.Errorf("%w: sheet %q: %v", ErrMalformedDocument, sheet, err))
		}
		rowNumber := 0
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				_ = rows.Close()
				return nil, err
			}
			rowNumber++
			totalRows++
			if totalRows > limits.MaxRows {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook rows", ErrLimitExceeded))
			}
			columns, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: sheet %q row %d: %v", ErrMalformedDocument, sheet, rowNumber, err))
			}
			if uint32(len(columns)) > limits.MaxColumns {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook columns", ErrLimitExceeded))
			}
			totalCells += uint64(len(columns))
			if totalCells > limits.MaxCells {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: workbook cells", ErrLimitExceeded))
			}
			for index := range columns {
				columns[index] = strings.TrimSpace(columns[index])
			}
			lastColumn := len(columns)
			for lastColumn > 0 && columns[lastColumn-1] == "" {
				lastColumn--
			}
			if lastColumn == 0 {
				continue
			}
			columns = columns[:lastColumn]
			start, err := excelize.CoordinatesToCellName(1, rowNumber)
			if err != nil {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: start cell: %v", ErrMalformedDocument, err))
			}
			end, err := excelize.CoordinatesToCellName(lastColumn, rowNumber)
			if err != nil {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, fmt.Errorf("%w: end cell: %v", ErrMalformedDocument, err))
			}
			sheetName := sheet
			if err := collector.add(strings.Join(columns, "\t"), nil, ContextLocatorV1{Kind: "xlsx_range", Sheet: &sheetName, CellStart: &start, CellEnd: &end}); err != nil {
				_ = rows.Close()
				return nil, parseError(FormatXLSX, err)
			}
		}
		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return nil, parseError(FormatXLSX, fmt.Errorf("%w: sheet %q: %v", ErrMalformedDocument, sheet, err))
		}
		if err := rows.Close(); err != nil {
			return nil, parseError(FormatXLSX, fmt.Errorf("%w: close sheet %q: %v", ErrMalformedDocument, sheet, err))
		}
	}
	return collector.result(FormatXLSX)
}
