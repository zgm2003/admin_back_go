package uploadtoken

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/infra/secretbox"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/shared/enum"
)

var ErrActiveCOSConfigUnavailable = errors.New("active COS upload config is unavailable")

type ObjectConfigProvider struct {
	repository Repository
	box        secretbox.Box
}

func NewObjectConfigProvider(repository Repository, box secretbox.Box) *ObjectConfigProvider {
	return &ObjectConfigProvider{repository: repository, box: box}
}

func (provider *ObjectConfigProvider) ActiveObjectConfig(ctx context.Context) (storagecos.ObjectConfig, error) {
	if provider == nil || provider.repository == nil {
		return storagecos.ObjectConfig{}, ErrRepositoryNotConfigured
	}
	config, err := provider.repository.GetEnabledConfig(ctx)
	if err != nil {
		return storagecos.ObjectConfig{}, err
	}
	if config == nil || config.SettingID <= 0 || strings.TrimSpace(config.Driver) != enum.UploadDriverCOS {
		return storagecos.ObjectConfig{}, ErrActiveCOSConfigUnavailable
	}
	secretID, err := provider.box.Decrypt(config.SecretIDEnc)
	if err != nil {
		return storagecos.ObjectConfig{}, err
	}
	secretKey, err := provider.box.Decrypt(config.SecretKeyEnc)
	if err != nil {
		return storagecos.ObjectConfig{}, err
	}
	if strings.TrimSpace(secretID) == "" || strings.TrimSpace(secretKey) == "" {
		return storagecos.ObjectConfig{}, ErrActiveCOSConfigUnavailable
	}
	return storagecos.ObjectConfig{
		SecretID: strings.TrimSpace(secretID), SecretKey: strings.TrimSpace(secretKey),
		Bucket: strings.TrimSpace(config.Bucket), Region: strings.TrimSpace(config.Region),
		Endpoint: strings.TrimSpace(config.Endpoint), BucketDomain: strings.TrimSpace(config.BucketDomain),
	}, nil
}
