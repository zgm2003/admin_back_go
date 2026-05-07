package clientversion

import (
	"context"
	"errors"
	"fmt"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
)

var ErrUploadConfigNotConfigured = errors.New("client version upload config is not configured")

type ManifestCOSPublisher struct {
	repository UploadConfigRepository
	box        secretbox.Box
	writer     storagecos.ObjectWriter
}

func NewManifestPublisher(repository UploadConfigRepository, box secretbox.Box, writer storagecos.ObjectWriter) *ManifestCOSPublisher {
	return &ManifestCOSPublisher{repository: repository, box: box, writer: writer}
}

func (p *ManifestCOSPublisher) Publish(ctx context.Context, platform string, body []byte) error {
	if p == nil || p.repository == nil || p.writer == nil {
		return ErrPublisherNotConfigured
	}
	cfg, err := p.repository.GetEnabledConfig(ctx)
	if err != nil {
		return fmt.Errorf("load upload config: %w", err)
	}
	if cfg == nil {
		return ErrUploadConfigNotConfigured
	}
	if cfg.Driver != enum.UploadDriverCOS {
		return fmt.Errorf("manifest publish only supports cos driver: %s", cfg.Driver)
	}
	secretID, err := p.box.Decrypt(cfg.SecretIDEnc)
	if err != nil || secretID == "" {
		return fmt.Errorf("decrypt cos secret id: %w", err)
	}
	secretKey, err := p.box.Decrypt(cfg.SecretKeyEnc)
	if err != nil || secretKey == "" {
		return fmt.Errorf("decrypt cos secret key: %w", err)
	}
	return p.writer.Put(ctx, storagecos.PutInput{
		SecretID:    secretID,
		SecretKey:   secretKey,
		Bucket:      cfg.Bucket,
		Region:      cfg.Region,
		Endpoint:    cfg.Endpoint,
		Key:         fmt.Sprintf("tauri_updater/%s.json", platform),
		Body:        body,
		ContentType: "application/json; charset=utf-8",
	})
}
