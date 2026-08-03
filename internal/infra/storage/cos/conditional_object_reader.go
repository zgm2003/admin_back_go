package cos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/storage"
)

type ConditionalObjectReader struct {
	streamer *COSObjectStreamer
}

func NewConditionalObjectReader(provider ObjectConfigProvider, config ObjectStreamerConfig) *ConditionalObjectReader {
	return &ConditionalObjectReader{streamer: NewObjectStreamer(provider, config)}
}

func (reader *ConditionalObjectReader) Head(ctx context.Context, input storage.ConditionalObjectInput) (storage.ConditionalObjectMetadata, error) {
	if err := validateConditionalObjectInput(input); err != nil {
		return storage.ConditionalObjectMetadata{}, err
	}
	metadata, err := reader.streamer.headObject(ctx, input.ObjectKey, input.ETag, input.Size)
	if err != nil {
		return storage.ConditionalObjectMetadata{}, mapConditionalObjectError(err)
	}
	return conditionalMetadata(metadata), nil
}

func (reader *ConditionalObjectReader) Open(ctx context.Context, input storage.ConditionalObjectInput) (io.ReadCloser, storage.ConditionalObjectMetadata, error) {
	if err := validateConditionalObjectInput(input); err != nil {
		return nil, storage.ConditionalObjectMetadata{}, err
	}
	body, metadata, err := reader.streamer.openObject(ctx, input.ObjectKey, input.ETag, input.Size)
	if err != nil {
		return nil, storage.ConditionalObjectMetadata{}, mapConditionalObjectError(err)
	}
	return body, conditionalMetadata(metadata), nil
}

func validateConditionalObjectInput(input storage.ConditionalObjectInput) error {
	if err := input.Validate(); err != nil || strings.TrimSpace(input.StorageProvider) != "cos" {
		return storage.ErrInvalidConditionalObjectInput
	}
	return nil
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
