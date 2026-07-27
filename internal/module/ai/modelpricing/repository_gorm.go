package modelpricing

import (
	"context"
	"errors"

	"admin_back_go/internal/infra/database"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (repository *GormRepository) FindOverride(ctx context.Context, catalogVendor string, modelID string) (*PriceOverride, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return findOverride(repository.db.WithContext(ctx), catalogVendor, modelID, false)
}

func (repository *GormRepository) ReplaceOverride(ctx context.Context, command ReplaceOverrideCommand, validateExisting ExistingOverrideValidator) (*PriceOverride, *PriceOverride, error) {
	if repository == nil || repository.db == nil {
		return nil, nil, ErrRepositoryNotConfigured
	}
	var before *PriceOverride
	var after *PriceOverride
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := findOverride(tx, command.CatalogVendor, command.ModelID, true)
		if err != nil {
			return err
		}
		if command.ExpectedVersion == 0 {
			if current != nil {
				return ErrVersionConflict
			}
			created := overrideFromCommand(command, 1, 0)
			if err := tx.Omit("Rates").Create(created).Error; err != nil {
				if isDuplicateKey(err) {
					return ErrVersionConflict
				}
				return err
			}
			setOverrideID(created.Rates, created.ID)
			if err := tx.Create(&created.Rates).Error; err != nil {
				return err
			}
			after = created
			return nil
		}
		if current == nil || current.Version != command.ExpectedVersion {
			return ErrVersionConflict
		}
		if validateExisting != nil {
			if err := validateExisting(cloneOverride(current)); err != nil {
				return err
			}
		}

		result := tx.Model(&PriceOverride{}).
			Where("id = ? AND version = ?", current.ID, command.ExpectedVersion).
			Updates(map[string]any{
				"version":     gorm.Expr("version + 1"),
				"source_url":  command.SourceURL,
				"verified_at": command.VerifiedAt,
				"updated_by":  command.UpdatedBy,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		if err := tx.Where("override_id = ?", current.ID).Delete(&PriceOverrideRate{}).Error; err != nil {
			return err
		}

		updated := overrideFromCommand(command, current.Version+1, current.ID)
		updated.CreatedAt = current.CreatedAt
		setOverrideID(updated.Rates, current.ID)
		if err := tx.Create(&updated.Rates).Error; err != nil {
			return err
		}
		before = current
		after = updated
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return cloneOverride(before), cloneOverride(after), nil
}

func (repository *GormRepository) DeleteOverride(ctx context.Context, command DeleteOverrideCommand, validateExisting ExistingOverrideValidator) (*PriceOverride, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if command.ExpectedVersion == 0 {
		return nil, ErrVersionConflict
	}
	var before *PriceOverride
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := findOverride(tx, command.CatalogVendor, command.ModelID, true)
		if err != nil {
			return err
		}
		if current == nil || current.Version != command.ExpectedVersion {
			return ErrVersionConflict
		}
		if validateExisting != nil {
			if err := validateExisting(cloneOverride(current)); err != nil {
				return err
			}
		}
		result := tx.Where("id = ? AND version = ?", current.ID, command.ExpectedVersion).Delete(&PriceOverride{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVersionConflict
		}
		before = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cloneOverride(before), nil
}

func findOverride(db *gorm.DB, catalogVendor string, modelID string, locked bool) (*PriceOverride, error) {
	query := db.Where("catalog_vendor = ? AND model_id = ?", catalogVendor, modelID).Order("id ASC").Limit(2)
	if locked {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []PriceOverride
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, ErrOverrideMappingAmbiguous
	}
	row := rows[0]
	if err := db.Where("override_id = ?", row.ID).
		Order("category ASC, unit ASC, tier_key ASC, id ASC").
		Find(&row.Rates).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func overrideFromCommand(command ReplaceOverrideCommand, version uint64, id uint64) *PriceOverride {
	return &PriceOverride{
		ID: id, CatalogVendor: command.CatalogVendor, ModelID: command.ModelID, Version: version,
		SourceURL: command.SourceURL, VerifiedAt: command.VerifiedAt, UpdatedBy: command.UpdatedBy,
		Rates: append([]PriceOverrideRate(nil), command.Rates...),
	}
}

func setOverrideID(rates []PriceOverrideRate, overrideID uint64) {
	for index := range rates {
		rates[index].ID = 0
		rates[index].OverrideID = overrideID
	}
}

func cloneOverride(row *PriceOverride) *PriceOverride {
	if row == nil {
		return nil
	}
	clone := *row
	clone.Rates = append([]PriceOverrideRate(nil), row.Rates...)
	return &clone
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
