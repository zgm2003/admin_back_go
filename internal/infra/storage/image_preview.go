package storage

import (
	"context"
	"errors"
	"mime"
	"strings"
)

var ErrInvalidImagePreviewInput = errors.New("invalid image preview input")

type ImagePreviewInput struct {
	StorageProvider string
	ObjectKey       string
	ETag            string
	Size            int64
	MIMEType        string
}

func (input ImagePreviewInput) Validate() error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(input.MIMEType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "image/") ||
		strings.TrimSpace(input.StorageProvider) == "" || strings.TrimSpace(input.ObjectKey) == "" ||
		strings.TrimSpace(input.ETag) == "" || input.Size <= 0 {
		return ErrInvalidImagePreviewInput
	}
	return nil
}

type ImagePreview struct {
	URL       string
	ExpiresIn int64
}

type ImagePreviewer interface {
	Preview(context.Context, ImagePreviewInput) (ImagePreview, error)
}
