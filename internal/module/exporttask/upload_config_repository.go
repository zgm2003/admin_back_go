package exporttask

import (
	"context"
	"errors"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrUploadConfigNotConfigured = errors.New("export task upload config is not configured")

type UploadConfig struct {
	SettingID    int64
	Driver       string
	SecretIDEnc  string
	SecretKeyEnc string
	Bucket       string
	Region       string
	AppID        string
	Endpoint     string
	BucketDomain string
}

type UploadConfigRepository interface {
	GetEnabledConfig(ctx context.Context) (*UploadConfig, error)
}

type GormUploadConfigRepository struct {
	db *gorm.DB
}

func NewGormUploadConfigRepository(client *database.Client) *GormUploadConfigRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormUploadConfigRepository{db: client.Gorm}
}

func (r *GormUploadConfigRepository) GetEnabledConfig(ctx context.Context) (*UploadConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row UploadConfig
	err := r.db.WithContext(ctx).
		Table("upload_setting AS s").
		Select(`s.id AS setting_id,
			d.driver, d.secret_id_enc, d.secret_key_enc, d.bucket, d.region, d.appid, d.endpoint, d.bucket_domain`).
		Joins("JOIN upload_driver AS d ON d.id = s.driver_id AND d.is_del = ?", enum.CommonNo).
		Joins("JOIN upload_rule AS rule ON rule.id = s.rule_id AND rule.is_del = ?", enum.CommonNo).
		Where("s.status = ?", enum.CommonYes).
		Where("s.is_del = ?", enum.CommonNo).
		Order("s.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.SettingID == 0 {
		return nil, nil
	}
	return &row, nil
}
