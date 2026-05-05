package uploadtoken

import (
	"context"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

type Repository interface {
	GetEnabledConfig(ctx context.Context) (*EnabledConfig, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) GetEnabledConfig(ctx context.Context) (*EnabledConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}

	var row EnabledConfig
	err := r.db.WithContext(ctx).
		Table("upload_setting AS s").
		Select(`s.id AS setting_id, s.driver_id, s.rule_id,
			d.driver, d.secret_id_enc, d.secret_key_enc, d.bucket, d.region, d.appid, d.endpoint, d.bucket_domain, d.role_arn,
			rule.max_size_mb, rule.image_exts, rule.file_exts`).
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
