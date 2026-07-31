package cos

import (
	"bufio"
	"compress/lzw"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

func verifyStaticGIFObject(ctx context.Context, client *tencentcos.Client, key, etag string, size int64) error {
	if client == nil || size <= 0 || strings.TrimSpace(etag) == "" {
		return ErrInvalidGIF
	}
	headers := make(http.Header)
	headers.Set("If-Match", etag)
	response, err := client.Object.Get(ctx, key, &tencentcos.ObjectGetOptions{XOptionHeader: &headers})
	if err != nil {
		return fmt.Errorf("cos GIF object get: %w", err)
	}
	if response == nil || response.Response == nil || response.Body == nil {
		return ErrInvalidGIF
	}
	defer response.Body.Close()
	if strings.TrimSpace(response.Header.Get("ETag")) != etag {
		return ErrInvalidGIF
	}
	contentLength := response.ContentLength
	if raw := strings.TrimSpace(response.Header.Get("Content-Length")); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return ErrInvalidGIF
		}
		contentLength = parsed
	}
	if contentLength != size {
		return ErrInvalidGIF
	}
	limited := &io.LimitedReader{R: response.Body, N: size}
	if err := requireStaticGIF(limited); err != nil {
		return err
	}
	if limited.N != 0 {
		return ErrInvalidGIF
	}
	return nil
}

func requireStaticGIF(source io.Reader) error {
	reader := bufio.NewReader(source)
	header := make([]byte, 13)
	if _, err := io.ReadFull(reader, header); err != nil {
		return ErrInvalidGIF
	}
	if signature := string(header[:6]); signature != "GIF87a" && signature != "GIF89a" {
		return ErrInvalidGIF
	}
	logicalWidth := uint32(binary.LittleEndian.Uint16(header[6:8]))
	logicalHeight := uint32(binary.LittleEndian.Uint16(header[8:10]))
	if logicalWidth == 0 || logicalHeight == 0 {
		return ErrInvalidGIF
	}
	globalColorEntries := 0
	if header[10]&0x80 != 0 {
		globalColorEntries = 1 << ((header[10] & 0x07) + 1)
		colorTableBytes := int64(3 * globalColorEntries)
		if err := discardGIFBytes(reader, colorTableBytes); err != nil {
			return ErrInvalidGIF
		}
	}
	frames := 0
	for {
		marker, err := reader.ReadByte()
		if err != nil {
			return ErrInvalidGIF
		}
		switch marker {
		case 0x21:
			if _, err := reader.ReadByte(); err != nil || skipGIFSubBlocks(reader) != nil {
				return ErrInvalidGIF
			}
		case 0x2c:
			frames++
			if frames > 1 {
				return ErrAnimatedGIF
			}
			descriptor := make([]byte, 9)
			if _, err := io.ReadFull(reader, descriptor); err != nil {
				return ErrInvalidGIF
			}
			left := uint32(binary.LittleEndian.Uint16(descriptor[0:2]))
			top := uint32(binary.LittleEndian.Uint16(descriptor[2:4]))
			width := uint32(binary.LittleEndian.Uint16(descriptor[4:6]))
			height := uint32(binary.LittleEndian.Uint16(descriptor[6:8]))
			if width == 0 || height == 0 || left+width > logicalWidth || top+height > logicalHeight {
				return ErrInvalidGIF
			}
			colorEntries := globalColorEntries
			if descriptor[8]&0x80 != 0 {
				colorEntries = 1 << ((descriptor[8] & 0x07) + 1)
				colorTableBytes := int64(3 * colorEntries)
				if err := discardGIFBytes(reader, colorTableBytes); err != nil {
					return ErrInvalidGIF
				}
			}
			if colorEntries == 0 {
				return ErrInvalidGIF
			}
			literalWidth, err := reader.ReadByte()
			if err != nil || literalWidth < 2 || literalWidth > 8 {
				return ErrInvalidGIF
			}
			if err := validateGIFImageData(reader, literalWidth, uint64(width)*uint64(height), colorEntries); err != nil {
				return ErrInvalidGIF
			}
		case 0x3b:
			if frames != 1 {
				return ErrInvalidGIF
			}
			if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
				return ErrInvalidGIF
			}
			return nil
		default:
			return ErrInvalidGIF
		}
	}
}

func validateGIFImageData(source *bufio.Reader, literalWidth byte, pixelCount uint64, colorEntries int) error {
	blocks := &gifImageBlockReader{source: source}
	decoder := lzw.NewReader(blocks, lzw.LSB, int(literalWidth))
	defer decoder.Close()

	var pixels [4096]byte
	for remaining := pixelCount; remaining > 0; {
		chunkSize := uint64(len(pixels))
		if remaining < chunkSize {
			chunkSize = remaining
		}
		chunk := pixels[:int(chunkSize)]
		if _, err := io.ReadFull(decoder, chunk); err != nil {
			return err
		}
		for _, pixel := range chunk {
			if int(pixel) >= colorEntries {
				return ErrInvalidGIF
			}
		}
		remaining -= chunkSize
	}

	var extra [1]byte
	if count, err := decoder.Read(extra[:]); count != 0 || !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return ErrInvalidGIF
	}
	return blocks.finish()
}

type gifImageBlockReader struct {
	source     *bufio.Reader
	buffer     [255]byte
	offset     int
	size       int
	terminated bool
	err        error
}

func (reader *gifImageBlockReader) ReadByte() (byte, error) {
	if reader.offset == reader.size {
		if err := reader.fill(); err != nil {
			return 0, err
		}
	}
	value := reader.buffer[reader.offset]
	reader.offset++
	return value, nil
}

func (reader *gifImageBlockReader) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	if reader.offset == reader.size {
		if err := reader.fill(); err != nil {
			return 0, err
		}
	}
	count := copy(target, reader.buffer[reader.offset:reader.size])
	reader.offset += count
	return count, nil
}

func (reader *gifImageBlockReader) fill() error {
	if reader.err != nil {
		return reader.err
	}
	if reader.terminated {
		return io.EOF
	}
	size, err := reader.source.ReadByte()
	if err != nil {
		reader.err = err
		return err
	}
	if size == 0 {
		reader.terminated = true
		reader.err = io.EOF
		return io.EOF
	}
	reader.offset = 0
	reader.size = int(size)
	if _, err := io.ReadFull(reader.source, reader.buffer[:reader.size]); err != nil {
		reader.size = 0
		reader.err = err
		return err
	}
	return nil
}

func (reader *gifImageBlockReader) finish() error {
	if reader.offset != reader.size {
		return ErrInvalidGIF
	}
	if reader.terminated {
		return nil
	}
	if reader.err != nil {
		return reader.err
	}
	size, err := reader.source.ReadByte()
	if err != nil {
		return err
	}
	if size != 0 {
		return ErrInvalidGIF
	}
	reader.terminated = true
	reader.err = io.EOF
	return nil
}

func skipGIFSubBlocks(reader *bufio.Reader) error {
	for {
		size, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if size == 0 {
			return nil
		}
		if err := discardGIFBytes(reader, int64(size)); err != nil {
			return err
		}
	}
}

func discardGIFBytes(reader io.Reader, count int64) error {
	read, err := io.CopyN(io.Discard, reader, count)
	if err != nil || read != count {
		return ErrInvalidGIF
	}
	return nil
}
