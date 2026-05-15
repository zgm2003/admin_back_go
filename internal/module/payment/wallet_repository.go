package payment

import (
	"context"
	"errors"

	"admin_back_go/internal/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) GetWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Wallet
	err := r.db.WithContext(ctx).Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	wallet, err := r.GetWallet(ctx, userID)
	if err != nil || wallet != nil {
		return wallet, err
	}
	row := Wallet{UserID: userID, IsDel: enum.CommonNo}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func lockWalletForUpdate(tx *gorm.DB, userID int64) (*Wallet, error) {
	var wallet Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND is_del = ?", userID, enum.CommonNo).
		First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}
