package payment

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestGetOrCreateWalletReturnsExistingOnDuplicateUserWallet(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ?")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_wallets`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry '7' for key 'uk_user_wallet_user'"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ?")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonNo, now, now))

	wallet, err := repo.GetOrCreateWallet(context.Background(), 7)
	if err != nil {
		t.Fatalf("expected duplicate wallet race to return existing wallet, got err=%v", err)
	}
	if wallet == nil || wallet.ID != 1 || wallet.UserID != 7 {
		t.Fatalf("expected existing wallet after duplicate race, got %#v", wallet)
	}
	assertPaymentMockExpectations(t, mock)
}
