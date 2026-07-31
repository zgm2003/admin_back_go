package cos

import (
	"bufio"
	"context"
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
	if header[10]&0x80 != 0 {
		colorTableBytes := int64(3 * (1 << ((header[10] & 0x07) + 1)))
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
			if descriptor[8]&0x80 != 0 {
				colorTableBytes := int64(3 * (1 << ((descriptor[8] & 0x07) + 1)))
				if err := discardGIFBytes(reader, colorTableBytes); err != nil {
					return ErrInvalidGIF
				}
			}
			if _, err := reader.ReadByte(); err != nil || skipGIFSubBlocks(reader) != nil {
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
