package cos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/storage"
)

type ConditionalObjectReader struct {
	streamer   *COSObjectStreamer
	previewTTL time.Duration
}

const maxConditionalObjectPreviewTTL = 5 * time.Minute

func NewConditionalObjectReader(provider ObjectConfigProvider, config ObjectStreamerConfig) *ConditionalObjectReader {
	previewTTL := config.PreviewTTL
	if previewTTL <= 0 || previewTTL > maxConditionalObjectPreviewTTL {
		previewTTL = maxConditionalObjectPreviewTTL
	}
	return &ConditionalObjectReader{streamer: NewObjectStreamer(provider, config), previewTTL: previewTTL}
}

func (reader *ConditionalObjectReader) Head(ctx context.Context, input storage.ConditionalObjectInput) (storage.ConditionalObjectMetadata, error) {
	input, err := normalizeConditionalObjectInput(input)
	if err != nil {
		return storage.ConditionalObjectMetadata{}, err
	}
	metadata, err := reader.streamer.headObject(ctx, input.ObjectKey, input.ETag, input.Size)
	if err != nil {
		return storage.ConditionalObjectMetadata{}, mapConditionalObjectError(err)
	}
	return conditionalMetadata(metadata), nil
}

func (reader *ConditionalObjectReader) Open(ctx context.Context, input storage.ConditionalObjectInput) (io.ReadCloser, storage.ConditionalObjectMetadata, error) {
	input, err := normalizeConditionalObjectInput(input)
	if err != nil {
		return nil, storage.ConditionalObjectMetadata{}, err
	}
	body, metadata, err := reader.streamer.openObject(ctx, input.ObjectKey, input.ETag, input.Size)
	if err != nil {
		return nil, storage.ConditionalObjectMetadata{}, mapConditionalObjectError(err)
	}
	return body, conditionalMetadata(metadata), nil
}

func (reader *ConditionalObjectReader) Preview(ctx context.Context, input storage.ConditionalObjectPreviewInput) (storage.ConditionalObjectPreview, error) {
	if reader == nil || reader.streamer == nil {
		return storage.ConditionalObjectPreview{}, ErrObjectStreamerNotConfigured
	}
	if err := input.Validate(); err != nil || strings.TrimSpace(input.Object.StorageProvider) != "cos" {
		return storage.ConditionalObjectPreview{}, storage.ErrInvalidConditionalObjectPreview
	}
	objectKey, err := TrustedAIContextDocumentObjectKey(input.Object.ObjectKey)
	if err != nil {
		return storage.ConditionalObjectPreview{}, fmt.Errorf("validate COS context preview key: %w", storage.ErrInvalidConditionalObjectPreview)
	}
	client, err := reader.streamer.objectClient(ctx)
	if err != nil {
		return storage.ConditionalObjectPreview{}, err
	}
	object := input.Object
	objectKey = strings.TrimSpace(objectKey)
	metadata, err := conditionalHead(ctx, reader.streamer.timeout, client, objectKey, strings.TrimSpace(object.ETag), object.Size)
	if err != nil {
		return storage.ConditionalObjectPreview{}, mapConditionalObjectError(err)
	}
	wantedMIME, _, _ := mime.ParseMediaType(strings.TrimSpace(input.MIMEType))
	if !strings.EqualFold(metadata.MIMEType, wantedMIME) {
		return storage.ConditionalObjectPreview{}, fmt.Errorf("verify COS context preview MIME type: %w", storage.ErrConditionalObjectVersionChanged)
	}
	ttl := reader.previewTTL
	if ttl <= 0 || ttl > maxConditionalObjectPreviewTTL {
		ttl = maxConditionalObjectPreviewTTL
	}
	signedURL, err := client.Object.GetPresignedURL2(ctx, http.MethodGet, objectKey, ttl, nil)
	if err != nil {
		return storage.ConditionalObjectPreview{}, fmt.Errorf("sign COS context preview: %w", err)
	}
	if signedURL == nil || strings.TrimSpace(signedURL.String()) == "" {
		return storage.ConditionalObjectPreview{}, errors.New("sign COS context preview: empty URL")
	}
	return storage.ConditionalObjectPreview{URL: signedURL.String(), ExpiresIn: int64(ttl.Seconds()), Metadata: conditionalMetadata(metadata)}, nil
}

func validateConditionalObjectInput(input storage.ConditionalObjectInput) error {
	_, err := normalizeConditionalObjectInput(input)
	return err
}

func normalizeConditionalObjectInput(input storage.ConditionalObjectInput) (storage.ConditionalObjectInput, error) {
	if err := input.Validate(); err != nil || strings.TrimSpace(input.StorageProvider) != "cos" {
		return storage.ConditionalObjectInput{}, storage.ErrInvalidConditionalObjectInput
	}
	key, err := trustedAIContextSourceObjectKey(input.ObjectKey)
	if err != nil {
		return storage.ConditionalObjectInput{}, storage.ErrInvalidConditionalObjectInput
	}
	input.StorageProvider = strings.TrimSpace(input.StorageProvider)
	input.ObjectKey = key
	input.ETag = strings.TrimSpace(input.ETag)
	return input, nil
}

func conditionalMetadata(metadata infraai.PreparedFileObjectMetadata) storage.ConditionalObjectMetadata {
	return storage.ConditionalObjectMetadata{ETag: metadata.ETag, Size: metadata.Size, MIMEType: metadata.MIMEType}
}

func mapConditionalObjectError(err error) error {
	switch {
	case errors.Is(err, ErrObjectUnavailable):
		return fmt.Errorf("read COS conditional object: %w", storage.ErrConditionalObjectUnavailable)
	case errors.Is(err, ErrObjectVersionChanged):
		return fmt.Errorf("read COS conditional object: %w", storage.ErrConditionalObjectVersionChanged)
	default:
		return err
	}
}

var _ storage.ConditionalObjectReader = (*ConditionalObjectReader)(nil)
var _ storage.ConditionalObjectPreviewer = (*ConditionalObjectReader)(nil)
