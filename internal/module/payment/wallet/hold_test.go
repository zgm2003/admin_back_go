package wallet

import (
	"context"
	"gorm.io/gorm"
	"testing"
)

func TestReserveHoldRejectsNilOrRootTransaction(t *testing.T) {
	repo := &GormRepository{}
	if _, err := repo.ReserveHoldInTx(context.Background(), nil, ReserveHoldInput{UserID: 1, RunID: 2, AmountUnits: 1}); err != ErrRepositoryNotConfigured {
		t.Fatalf("nil repository handle error = %v", err)
	}
	repo = &GormRepository{db: &gorm.DB{}}
	if _, err := repo.ReserveHoldInTx(context.Background(), repo.db, ReserveHoldInput{UserID: 1, RunID: 2, AmountUnits: 1}); err != ErrHoldTransactionRequired {
		t.Fatalf("root transaction error = %v", err)
	}
}

func TestCaptureHoldRejectsInvalidSummary(t *testing.T) {
	if err := validateHoldSummary(""); err != ErrHoldSummaryInvalid {
		t.Fatalf("blank summary error = %v", err)
	}
	if err := validateHoldSummary("ok"); err != nil {
		t.Fatalf("valid summary error = %v", err)
	}
}
