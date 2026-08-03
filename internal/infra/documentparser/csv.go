package documentparser

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type csvParser struct{}

func (csvParser) Name() string    { return string(FormatCSV) }
func (csvParser) Version() string { return "1" }

func (csvParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatCSV)
	if err != nil {
		return nil, err
	}
	if err := rejectTextDisguise(data); err != nil {
		return nil, parseError(FormatCSV, err)
	}
	if !utf8.Valid(data) {
		return nil, parseError(FormatCSV, ErrInvalidEncoding)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	collector := newBlockCollector(limits)
	var rowCount uint32
	var cellCount uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, parseError(FormatCSV, fmt.Errorf("%w: %v", ErrMalformedDocument, err))
		}
		rowCount++
		if rowCount > limits.MaxRows || uint32(len(record)) > limits.MaxColumns {
			return nil, parseError(FormatCSV, fmt.Errorf("%w: CSV rows or columns", ErrLimitExceeded))
		}
		cellCount += uint64(len(record))
		if cellCount > limits.MaxCells {
			return nil, parseError(FormatCSV, fmt.Errorf("%w: CSV cells", ErrLimitExceeded))
		}
		for index := range record {
			record[index] = strings.TrimSpace(record[index])
		}
		row := rowCount
		if err := collector.add(strings.Join(record, "\t"), nil, ContextLocatorV1{Kind: "csv_rows", RowStart: &row, RowEnd: &row}); err != nil {
			return nil, parseError(FormatCSV, err)
		}
	}
	return collector.result(FormatCSV)
}
