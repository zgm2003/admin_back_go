package documentparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type txtParser struct{}

func (txtParser) Name() string    { return string(FormatTXT) }
func (txtParser) Version() string { return "1" }

func (txtParser) Parse(ctx context.Context, source Source, limits Limits) ([]Block, error) {
	data, err := readSource(ctx, source, limits, FormatTXT)
	if err != nil {
		return nil, err
	}
	if err := rejectBinarySignature(data); err != nil {
		return nil, parseError(FormatTXT, err)
	}
	text, err := decodeText(data)
	if err != nil {
		return nil, parseError(FormatTXT, err)
	}

	collector := newBlockCollector(limits)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lineNumber := uint32(index + 1)
		if err := collector.add(line, nil, ContextLocatorV1{Kind: "txt_line", LineStart: &lineNumber, LineEnd: &lineNumber}); err != nil {
			return nil, parseError(FormatTXT, err)
		}
	}
	return collector.result(FormatTXT)
}

func decodeText(data []byte) (string, error) {
	if len(data) >= 2 && ((data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff)) {
		if (len(data)-2)%2 != 0 {
			return "", ErrInvalidEncoding
		}
		var order binary.ByteOrder = binary.LittleEndian
		if data[0] == 0xfe {
			order = binary.BigEndian
		}
		units := make([]uint16, 0, (len(data)-2)/2)
		for offset := 2; offset < len(data); offset += 2 {
			units = append(units, order.Uint16(data[offset:offset+2]))
		}
		for index := 0; index < len(units); index++ {
			unit := units[index]
			if unit >= 0xd800 && unit <= 0xdbff {
				if index+1 >= len(units) || units[index+1] < 0xdc00 || units[index+1] > 0xdfff {
					return "", ErrInvalidEncoding
				}
				index++
			} else if unit >= 0xdc00 && unit <= 0xdfff {
				return "", ErrInvalidEncoding
			}
		}
		value := string(utf16.Decode(units))
		if strings.IndexByte(value, 0) >= 0 {
			return "", ErrBinaryDisguise
		}
		return value, nil
	}
	if !utf8.Valid(data) {
		return "", ErrInvalidEncoding
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", ErrBinaryDisguise
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: UTF-8", ErrInvalidEncoding)
	}
	return string(data), nil
}
