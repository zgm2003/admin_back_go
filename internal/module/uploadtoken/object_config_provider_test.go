package uploadtoken

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/enum"
)

type inspectionConfigRepository struct {
	config *EnabledConfig
	err    error
}

func (repository inspectionConfigRepository) GetEnabledConfig(context.Context) (*EnabledConfig, error) {
	return repository.config, repository.err
}

func TestObjectConfigProviderUsesCurrentEnabledCOSConfig(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, err := box.Encrypt("secret-id")
	if err != nil {
		t.Fatal(err)
	}
	secretKey, err := box.Encrypt("secret-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := NewObjectConfigProvider(inspectionConfigRepository{config: &EnabledConfig{
		SettingID: 3, Driver: enum.UploadDriverCOS, SecretIDEnc: secretID, SecretKeyEnc: secretKey,
		Bucket: "bucket-1", Region: "ap-test", Endpoint: "https://cos.test", BucketDomain: "cdn.test",
	}}, box)

	config, err := provider.ActiveObjectConfig(context.Background())
	if err != nil {
		t.Fatalf("ActiveObjectConfig: %v", err)
	}
	if config.SecretID != "secret-id" || config.SecretKey != "secret-key" || config.Bucket != "bucket-1" ||
		config.Region != "ap-test" || config.Endpoint != "https://cos.test" || config.BucketDomain != "cdn.test" {
		t.Fatalf("object config=%#v", config)
	}
}
