package cos

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"admin_back_go/internal/infra/storage"
)

const defaultImagePreviewTTL = 5 * time.Minute

type ImagePreviewerConfig struct {
	Enabled    bool
	Timeout    time.Duration
	TTL        time.Duration
	HTTPClient *http.Client
}

type COSImagePreviewer struct {
	streamer *COSObjectStreamer
	ttl      time.Duration
}

func NewImagePreviewer(provider ObjectConfigProvider, config ImagePreviewerConfig) *COSImagePreviewer {
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.TTL <= 0 || config.TTL > defaultImagePreviewTTL {
		config.TTL = defaultImagePreviewTTL
	}
	return &COSImagePreviewer{
		streamer: NewObjectStreamer(provider, ObjectStreamerConfig{
			Enabled: config.Enabled, Timeout: config.Timeout, HTTPClient: config.HTTPClient,
		}),
		ttl: config.TTL,
	}
}

func (previewer *COSImagePreviewer) Preview(ctx context.Context, input storage.ImagePreviewInput) (storage.ImagePreview, error) {
	if err := input.Validate(); err != nil || strings.TrimSpace(input.StorageProvider) != "cos" {
		return storage.ImagePreview{}, storage.ErrInvalidImagePreviewInput
	}
	objectKey, err := TrustedAIChatObjectKey(input.ObjectKey, "image")
	if err != nil {
		return storage.ImagePreview{}, fmt.Errorf("validate COS image preview key: %w", storage.ErrInvalidImagePreviewInput)
	}
	client, err := previewer.streamer.objectClient(ctx)
	if err != nil {
		return storage.ImagePreview{}, err
	}
	metadata, err := conditionalHead(ctx, previewer.streamer.timeout, client, objectKey, strings.TrimSpace(input.ETag), input.Size)
	if err != nil {
		return storage.ImagePreview{}, mapConditionalObjectError(err)
	}
	wantedMIME, _, _ := mime.ParseMediaType(strings.TrimSpace(input.MIMEType))
	if !strings.EqualFold(metadata.MIMEType, wantedMIME) {
		return storage.ImagePreview{}, fmt.Errorf("verify COS image preview MIME type: %w", storage.ErrConditionalObjectVersionChanged)
	}
	signedURL, err := client.Object.GetPresignedURL2(ctx, http.MethodGet, objectKey, previewer.ttl, nil)
	if err != nil {
		return storage.ImagePreview{}, fmt.Errorf("sign COS image preview: %w", err)
	}
	if signedURL == nil || strings.TrimSpace(signedURL.String()) == "" {
		return storage.ImagePreview{}, errors.New("sign COS image preview: empty URL")
	}
	return storage.ImagePreview{URL: signedURL.String(), ExpiresIn: int64(previewer.ttl.Seconds())}, nil
}

var _ storage.ImagePreviewer = (*COSImagePreviewer)(nil)
