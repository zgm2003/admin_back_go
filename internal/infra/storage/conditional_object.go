package storage

import (
	"context"
	"errors"
	"io"
	"strings"
)

var (
	ErrInvalidConditionalObjectInput   = errors.New("invalid conditional object input")
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
