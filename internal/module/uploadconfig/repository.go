package uploadconfig

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	ListDrivers(ctx context.Context, query DriverListQuery) ([]Driver, int64, error)
	ExistsDriverBucket(ctx context.Context, driver string, bucket string, excludeID int64) (bool, error)
	CreateDriver(ctx context.Context, row Driver) (int64, error)
	GetDriver(ctx context.Context, id int64) (*Driver, error)
	UpdateDriver(ctx context.Context, id int64, fields map[string]any) error
	DeleteDrivers(ctx context.Context, ids []int64) error
	DriverReferenced(ctx context.Context, ids []int64) (bool, error)
	ListRules(ctx context.Context, query RuleListQuery) ([]Rule, int64, error)
	ExistsRuleTitle(ctx context.Context, title string, excludeID int64) (bool, error)
	CreateRule(ctx context.Context, row Rule) (int64, error)
	GetRule(ctx context.Context, id int64) (*Rule, error)
	UpdateRule(ctx context.Context, id int64, fields map[string]any) error
	DeleteRules(ctx context.Context, ids []int64) error
	RuleReferenced(ctx context.Context, ids []int64) (bool, error)
	ListSettings(ctx context.Context, query SettingListQuery) ([]SettingListRow, int64, error)
	DriverDict(ctx context.Context) ([]Driver, error)
	RuleDict(ctx context.Context) ([]Rule, error)
	DriverExists(ctx context.Context, id int64) (bool, error)
	RuleExists(ctx context.Context, id int64) (bool, error)
	ExistsSettingDriverRule(ctx context.Context, driverID int64, ruleID int64, excludeID int64) (bool, error)
	CreateSetting(ctx context.Context, row Setting) (int64, error)
	GetSetting(ctx context.Context, id int64) (*Setting, error)
	UpdateSetting(ctx context.Context, id int64, fields map[string]any) error
	EnableSettingExclusive(ctx context.Context, id int64, row Setting, updateExisting bool) (int64, error)
	SettingEnabledIn(ctx context.Context, ids []int64) (bool, error)
	DeleteSettings(ctx context.Context, ids []int64) error
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

func (r *GormRepository) ListDrivers(ctx context.Context, query DriverListQuery) ([]Driver, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Driver{}).Where("is_del = ?", enum.CommonNo)
	driver := strings.TrimSpace(query.Driver)
	if driver != "" {
		db = db.Where("driver = ?", driver)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Driver
	err := db.Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) ExistsDriverBucket(ctx context.Context, driver string, bucket string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Driver{}).
		Where("driver = ?", driver).
		Where("bucket = ?", bucket).
		Where("is_del = ?", enum.CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CreateDriver(ctx context.Context, row Driver) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) GetDriver(ctx context.Context, id int64) (*Driver, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Driver
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateDriver(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Driver{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) DeleteDrivers(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Driver{}).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) DriverReferenced(ctx context.Context, ids []int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("driver_id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) ListRules(ctx context.Context, query RuleListQuery) ([]Rule, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}

	db := r.db.WithContext(ctx).Model(&Rule{}).Where("is_del = ?", enum.CommonNo)
	title := strings.TrimSpace(query.Title)
	if title != "" {
		db = db.Where("title LIKE ?", title+"%")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Rule
	err := db.Order("id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) ExistsRuleTitle(ctx context.Context, title string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Model(&Rule{}).Where("title = ?", title).Where("is_del = ?", enum.CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CreateRule(ctx context.Context, row Rule) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) GetRule(ctx context.Context, id int64) (*Rule, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Rule
	err := r.db.WithContext(ctx).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateRule(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Rule{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) DeleteRules(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Rule{}).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}

func (r *GormRepository) RuleReferenced(ctx context.Context, ids []int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("rule_id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) ListSettings(ctx context.Context, query SettingListQuery) ([]SettingListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	base := r.db.WithContext(ctx).
		Table("upload_setting AS s").
		Where("s.is_del = ?", enum.CommonNo)
	remark := strings.TrimSpace(query.Remark)
	if remark != "" {
		base = base.Where("s.remark LIKE ?", remark+"%")
	}
	if query.Status != nil {
		base = base.Where("s.status = ?", *query.Status)
	}
	if query.DriverID != nil {
		base = base.Where("s.driver_id = ?", *query.DriverID)
	}
	if query.RuleID != nil {
		base = base.Where("s.rule_id = ?", *query.RuleID)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []SettingListRow
	err := base.
		Select("s.id, s.driver_id, s.rule_id, s.status, s.remark, s.created_at, s.updated_at, d.driver, d.bucket, rule.title AS rule_title").
		Joins("LEFT JOIN upload_driver AS d ON d.id = s.driver_id AND d.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN upload_rule AS rule ON rule.id = s.rule_id AND rule.is_del = ?", enum.CommonNo).
		Order("s.id desc").
		Limit(query.PageSize).
		Offset((query.CurrentPage - 1) * query.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *GormRepository) DriverDict(ctx context.Context) ([]Driver, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Driver
	err := r.db.WithContext(ctx).
		Where("is_del = ?", enum.CommonNo).
		Order("id desc").
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) RuleDict(ctx context.Context) ([]Rule, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []Rule
	err := r.db.WithContext(ctx).
		Where("is_del = ?", enum.CommonNo).
		Order("id desc").
		Find(&rows).Error
	return rows, err
}

func (r *GormRepository) DriverExists(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Driver{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) RuleExists(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Rule{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) ExistsSettingDriverRule(ctx context.Context, driverID int64, ruleID int64, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("driver_id = ?", driverID).
		Where("rule_id = ?", ruleID).
		Where("is_del = ?", enum.CommonNo)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GormRepository) CreateSetting(ctx context.Context, row Setting) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) GetSetting(ctx context.Context, id int64) (*Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Setting
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GormRepository) UpdateSetting(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("id = ?", id).
		Where("is_del = ?", enum.CommonNo).
		Updates(fields).Error
}

func (r *GormRepository) EnableSettingExclusive(ctx context.Context, id int64, row Setting, updateExisting bool) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	var savedID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked []Setting
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("is_del = ?", enum.CommonNo).
			Find(&locked).Error; err != nil {
			return err
		}
		if err := tx.Model(&Setting{}).
			Where("is_del = ?", enum.CommonNo).
			Where("status = ?", enum.CommonYes).
			Update("status", enum.CommonNo).Error; err != nil {
			return err
		}

		row.Status = enum.CommonYes
		if updateExisting {
			fields := map[string]any{"status": enum.CommonYes}
			if row.DriverID > 0 {
				fields["driver_id"] = row.DriverID
			}
			if row.RuleID > 0 {
				fields["rule_id"] = row.RuleID
			}
			if row.DriverID > 0 || row.RuleID > 0 {
				fields["remark"] = row.Remark
			}
			result := tx.Model(&Setting{}).
				Where("id = ?", id).
				Where("is_del = ?", enum.CommonNo).
				Updates(fields)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			savedID = id
			return nil
		}

		row.IsDel = enum.CommonNo
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		savedID = row.ID
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return savedID, err
}

func (r *GormRepository) SettingEnabledIn(ctx context.Context, ids []int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("id IN ?", ids).
		Where("status = ?", enum.CommonYes).
		Where("is_del = ?", enum.CommonNo).
		Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) DeleteSettings(ctx context.Context, ids []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&Setting{}).
		Where("id IN ?", ids).
		Where("is_del = ?", enum.CommonNo).
		Update("is_del", enum.CommonYes).Error
}
