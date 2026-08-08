package storage

import (
	"context"
	"errors"
	"io"
	"mime"
	"strings"
)

var (
	ErrInvalidConditionalObjectInput   = errors.New("invalid conditional object input")
	ErrInvalidConditionalObjectPreview = errors.New("invalid conditional object preview input")
	ErrConditionalObjectUnavailable    = errors.New("conditional object unavailable")
	ErrConditionalObjectVersionChanged = errors.New("conditional object version changed")
)

type ConditionalObjectInput struct {
	StorageProvider string
	ObjectKey       string
	ETag            string
	Size            int64
}

func (input ConditionalObjectInput) Validate() error {
	if strings.TrimSpace(input.StorageProvider) == "" || strings.TrimSpace(input.ObjectKey) == "" ||
		strings.TrimSpace(input.ETag) == "" || input.Size <= 0 {
		return ErrInvalidConditionalObjectInput
	}
	return nil
}

type ConditionalObjectMetadata struct {
	ETag     string
	Size     int64
	MIMEType string
}

type ConditionalObjectReader interface {
	Head(context.Context, ConditionalObjectInput) (ConditionalObjectMetadata, error)
	Open(context.Context, ConditionalObjectInput) (io.ReadCloser, ConditionalObjectMetadata, error)
}

// ConditionalObjectPreviewInput carries the persisted source facts needed to
// prove an object before issuing a short-lived browser URL.
type ConditionalObjectPreviewInput struct {
	Object   ConditionalObjectInput
	MIMEType string
}

func (input ConditionalObjectPreviewInput) Validate() error {
	if err := input.Object.Validate(); err != nil {
		return ErrInvalidConditionalObjectPreview
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(input.MIMEType))
	if err != nil || strings.TrimSpace(mediaType) == "" {
		return ErrInvalidConditionalObjectPreview
	}
	return nil
}

type ConditionalObjectPreview struct {
	URL       string
	ExpiresIn int64
	Metadata  ConditionalObjectMetadata
}

type ConditionalObjectPreviewer interface {
	Preview(context.Context, ConditionalObjectPreviewInput) (ConditionalObjectPreview, error)
}
