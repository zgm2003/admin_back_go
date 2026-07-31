package openaicompat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

var ErrMaterializedBodyTooLarge = errors.New("materialized chat request body is too large")

type MaterializedRequest struct {
	Body          io.ReadCloser
	ContentLength int64
	Result        <-chan MaterializationResult
}

type MaterializationResult struct {
	Metrics infraai.FileInputMetrics
	Err     error
}

type fileManifestSegment struct {
	literal []byte
	file    *infraai.PreparedFileRef
	prefix  []byte
	suffix  []byte
}

func FileManifestContentLength(manifest infraai.PreparedChatFileManifest) (int64, error) {
	_, length, err := compileFileManifestSegments(manifest)
	return length, err
}

func MaterializeFileManifest(ctx context.Context, manifest infraai.PreparedChatFileManifest, objects infraai.PreparedFileOpener) (MaterializedRequest, error) {
	if objects == nil {
		return MaterializedRequest{}, fmt.Errorf("%w: prepared file opener is missing", infraai.ErrInvalidConfig)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return MaterializedRequest{}, err
	}
	segments, length, err := compileFileManifestSegments(manifest)
	if err != nil {
		return MaterializedRequest{}, err
	}
	reader, writer := io.Pipe()
	results := make(chan MaterializationResult, 1)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.CloseWithError(ctx.Err())
			_ = writer.CloseWithError(ctx.Err())
		case <-done:
		}
	}()
	go func() {
		metrics, produceErr := writeFileManifestSegments(ctx, writer, segments, objects)
		metrics.MaterializedRequestBytes = length
		if produceErr != nil {
			_ = writer.CloseWithError(produceErr)
		} else {
			_ = writer.Close()
		}
		close(done)
		results <- MaterializationResult{
			Metrics: metrics, Err: produceErr,
		}
		close(results)
	}()
	return MaterializedRequest{Body: reader, ContentLength: length, Result: results}, nil
}

func compileFileManifestSegments(manifest infraai.PreparedChatFileManifest) ([]fileManifestSegment, int64, error) {
	if err := manifest.Validate(); err != nil {
		return nil, 0, err
	}
	var request bytes.Buffer
	if err := json.Compact(&request, manifest.Request); err != nil {
		return nil, 0, err
	}
	canonical := request.Bytes()
	segments := make([]fileManifestSegment, 0, len(manifest.Files)*2+1)
	cursor := 0
	var length int64
	for index := range manifest.Files {
		file := manifest.Files[index]
		partStart, partLength, err := locateFileRefPart(canonical[cursor:], file.Ref)
		if err != nil {
			return nil, 0, err
		}
		partStart += cursor
		if partStart > cursor {
			literal := append([]byte(nil), canonical[cursor:partStart]...)
			segments = append(segments, fileManifestSegment{literal: literal})
			length, err = addMaterializedLength(length, int64(len(literal)))
			if err != nil {
				return nil, 0, err
			}
		}
		prefix, suffix, err := materializedFilePartAffixes(file)
		if err != nil {
			return nil, 0, err
		}
		segments = append(segments, fileManifestSegment{file: &manifest.Files[index], prefix: prefix, suffix: suffix})
		dataLength, err := dataURLLength(file)
		if err != nil {
			return nil, 0, err
		}
		length, err = addMaterializedLength(length, int64(len(prefix)))
		if err == nil {
			length, err = addMaterializedLength(length, dataLength-int64(len("data:"+file.MIMEType+";base64,")))
		}
		if err == nil {
			length, err = addMaterializedLength(length, int64(len(suffix)))
		}
		if err != nil {
			return nil, 0, err
		}
		cursor = partStart + partLength
	}
	if cursor < len(canonical) {
		literal := append([]byte(nil), canonical[cursor:]...)
		segments = append(segments, fileManifestSegment{literal: literal})
		var err error
		length, err = addMaterializedLength(length, int64(len(literal)))
		if err != nil {
			return nil, 0, err
		}
	}
	return segments, length, nil
}

func locateFileRefPart(request []byte, ref string) (int, int, error) {
	candidates := make([][]byte, 0, 2)
	for _, part := range []any{
		struct {
			Type string `json:"type"`
			Ref  string `json:"ref"`
		}{Type: "file_ref", Ref: ref},
		struct {
			Ref  string `json:"ref"`
			Type string `json:"type"`
		}{Ref: ref, Type: "file_ref"},
	} {
		encoded, err := json.Marshal(part)
		if err != nil {
			return 0, 0, err
		}
		candidates = append(candidates, encoded)
	}
	position := -1
	length := 0
	for _, candidate := range candidates {
		if index := bytes.Index(request, candidate); index >= 0 && (position < 0 || index < position) {
			position = index
			length = len(candidate)
		}
	}
	if position < 0 {
		return 0, 0, fmt.Errorf("prepared file_ref %q is not canonical", ref)
	}
	return position, length, nil
}

func materializedFilePartAffixes(file infraai.PreparedFileRef) ([]byte, []byte, error) {
	const marker = "OPENAI_FILE_BASE64_PAYLOAD"
	prefix := "data:" + file.MIMEType + ";base64,"
	part := struct {
		Type string `json:"type"`
		File struct {
			Filename string `json:"filename"`
			FileData string `json:"file_data"`
		} `json:"file"`
	}{Type: "file"}
	part.File.Filename = file.Filename
	part.File.FileData = prefix + marker
	encoded, err := json.Marshal(part)
	if err != nil {
		return nil, nil, err
	}
	position := bytes.Index(encoded, []byte(marker))
	if position < 0 {
		return nil, nil, errors.New("materialized file marker was escaped")
	}
	return append([]byte(nil), encoded[:position]...), append([]byte(nil), encoded[position+len(marker):]...), nil
}

func writeFileManifestSegments(ctx context.Context, writer *io.PipeWriter, segments []fileManifestSegment, objects infraai.PreparedFileOpener) (infraai.FileInputMetrics, error) {
	var metrics infraai.FileInputMetrics
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return metrics, err
		}
		if segment.file == nil {
			if _, err := writer.Write(segment.literal); err != nil {
				return metrics, err
			}
			continue
		}
		if _, err := writer.Write(segment.prefix); err != nil {
			return metrics, err
		}
		input := infraai.PreparedFileOpenInput{ObjectKey: segment.file.ObjectKey, ETag: segment.file.ETag, Size: segment.file.Size}
		streamStartedAt := time.Now()
		body, metadata, err := objects.Open(ctx, input)
		if err != nil {
			metrics.COSStreamMS += time.Since(streamStartedAt).Milliseconds()
			return metrics, err
		}
		if !preparedFileMetadataMatches(*segment.file, metadata) {
			_ = body.Close()
			metrics.COSStreamMS += time.Since(streamStartedAt).Milliseconds()
			return metrics, errors.New("prepared file object metadata changed")
		}
		encoder := base64.NewEncoder(base64.StdEncoding, writer)
		_, copyErr := io.CopyN(encoder, body, segment.file.Size)
		encoderErr := encoder.Close()
		closeErr := body.Close()
		metrics.COSStreamMS += time.Since(streamStartedAt).Milliseconds()
		if copyErr != nil {
			return metrics, copyErr
		}
		if encoderErr != nil {
			return metrics, encoderErr
		}
		if closeErr != nil {
			return metrics, closeErr
		}
		if _, err := writer.Write(segment.suffix); err != nil {
			return metrics, err
		}
	}
	return metrics, nil
}

func preparedFileMetadataMatches(file infraai.PreparedFileRef, metadata infraai.PreparedFileObjectMetadata) bool {
	return metadata.ETag == file.ETag && metadata.Size == file.Size && metadata.MIMEType == file.MIMEType
}

func base64EncodedLength(size int64) (int64, error) {
	if size < 0 || size > (math.MaxInt64-2)/4*3 {
		return 0, ErrMaterializedBodyTooLarge
	}
	return 4 * ((size + 2) / 3), nil
}

func dataURLLength(file infraai.PreparedFileRef) (int64, error) {
	encoded, err := base64EncodedLength(file.Size)
	if err != nil {
		return 0, err
	}
	return addMaterializedLength(int64(len("data:"+file.MIMEType+";base64,")), encoded)
}

func addMaterializedLength(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, ErrMaterializedBodyTooLarge
	}
	return left + right, nil
}
