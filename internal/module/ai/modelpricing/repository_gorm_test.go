package modelpricing

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/pricing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAIModelPricingGormRepositoryFindDetectsAmbiguityAndLoadsRates(t *testing.T) {
	repository, mock, closeDB := newAIModelPricingMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	expectOverrideLookup(mock, false).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/pricing", now, 7, now, now))
	expectOverrideRates(mock).WillReturnRows(overrideRateRows().
		AddRow(1, 9, "input", "token", "", 125_000_000, 1_000_000).
		AddRow(2, 9, "output", "token", "", 350_000_000, 1_000_000))
	row, err := repository.FindOverride(context.Background(), "openai", "gpt-reviewed")
	if err != nil || row == nil || row.Version != 2 || len(row.Rates) != 2 {
		t.Fatalf("FindOverride = %#v, %v", row, err)
	}

	expectOverrideLookup(mock, false).WillReturnRows(overrideRows().
		AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/pricing", now, 7, now, now).
		AddRow(10, "openai", "gpt-reviewed", 3, "https://openai.com/pricing", now, 7, now, now))
	if row, err := repository.FindOverride(context.Background(), "openai", "gpt-reviewed"); row != nil || !errors.Is(err, ErrOverrideMappingAmbiguous) {
		t.Fatalf("ambiguous FindOverride = %#v, %v", row, err)
	}
	assertAIModelPricingSQLMock(t, mock)
}

func TestAIModelPricingGormRepositoryCreatesVersionOneInOneTransaction(t *testing.T) {
	repository, mock, closeDB := newAIModelPricingMockRepository(t)
	defer closeDB()
	command := validReplaceCommand(0)

	mock.ExpectBegin()
	expectOverrideLookup(mock, true).WillReturnRows(overrideRows())
	mock.ExpectExec("INSERT INTO `ai_model_price_overrides`").WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec("INSERT INTO `ai_model_price_override_rates`").WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	before, after, err := repository.ReplaceOverride(context.Background(), command, nil)
	if err != nil || before != nil || after == nil || after.ID != 9 || after.Version != 1 || len(after.Rates) != 2 {
		t.Fatalf("ReplaceOverride create = %#v, %#v, %v", before, after, err)
	}
	for _, rate := range after.Rates {
		if rate.OverrideID != 9 {
			t.Fatalf("created rate missing override id: %#v", rate)
		}
	}
	assertAIModelPricingSQLMock(t, mock)
}

func TestAIModelPricingGormRepositoryUpdateUsesCompareAndSwapThenReplacesRates(t *testing.T) {
	repository, mock, closeDB := newAIModelPricingMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectOverrideLookup(mock, true).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/old", now, 3, now, now))
	expectOverrideRates(mock).WillReturnRows(overrideRateRows().
		AddRow(1, 9, "input", "token", "", 100_000_000, 1_000_000).
		AddRow(2, 9, "output", "token", "", 200_000_000, 1_000_000))
	mock.ExpectExec("UPDATE `ai_model_price_overrides` SET .*`version`=version \\+ 1.*WHERE id = .* AND version =").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `ai_model_price_override_rates` WHERE override_id = ?")).WithArgs(uint64(9)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `ai_model_price_override_rates`").WillReturnResult(sqlmock.NewResult(3, 2))
	mock.ExpectCommit()

	before, after, err := repository.ReplaceOverride(context.Background(), validReplaceCommand(2), nil)
	if err != nil || before == nil || before.Version != 2 || after == nil || after.Version != 3 || len(after.Rates) != 2 {
		t.Fatalf("ReplaceOverride update = %#v, %#v, %v", before, after, err)
	}
	assertAIModelPricingSQLMock(t, mock)
}

func TestAIModelPricingGormRepositoryReturnsStableVersionConflict(t *testing.T) {
	t.Run("concurrent first write", func(t *testing.T) {
		repository, mock, closeDB := newAIModelPricingMockRepository(t)
		defer closeDB()
		mock.ExpectBegin()
		expectOverrideLookup(mock, true).WillReturnRows(overrideRows())
		mock.ExpectExec("INSERT INTO `ai_model_price_overrides`").WillReturnError(&mysqlDriver.MySQLError{
			Number: 1062, Message: "Duplicate entry 'openai-gpt-reviewed' for key 'uk_ai_model_price_overrides_identity'",
		})
		mock.ExpectRollback()
		if _, _, err := repository.ReplaceOverride(context.Background(), validReplaceCommand(0), nil); !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("concurrent create error = %v", err)
		}
		assertAIModelPricingSQLMock(t, mock)
	})

	t.Run("stale expected version", func(t *testing.T) {
		repository, mock, closeDB := newAIModelPricingMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectOverrideLookup(mock, true).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 3, "https://openai.com/pricing", now, 7, now, now))
		expectOverrideRates(mock).WillReturnRows(overrideRateRows())
		mock.ExpectRollback()
		if before, after, err := repository.ReplaceOverride(context.Background(), validReplaceCommand(2), nil); before != nil || after != nil || !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("stale ReplaceOverride = %#v, %#v, %v", before, after, err)
		}
		assertAIModelPricingSQLMock(t, mock)
	})

	t.Run("compare and swap lost", func(t *testing.T) {
		repository, mock, closeDB := newAIModelPricingMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectOverrideLookup(mock, true).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/pricing", now, 7, now, now))
		expectOverrideRates(mock).WillReturnRows(overrideRateRows())
		mock.ExpectExec("UPDATE `ai_model_price_overrides`").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		if _, _, err := repository.ReplaceOverride(context.Background(), validReplaceCommand(2), nil); !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("lost compare-and-swap error = %v", err)
		}
		assertAIModelPricingSQLMock(t, mock)
	})
}

func TestAIModelPricingGormRepositoryValidatesLockedExistingBeforeMutation(t *testing.T) {
	repository, mock, closeDB := newAIModelPricingMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	wantErr := errors.New("stored override is corrupt")

	mock.ExpectBegin()
	expectOverrideLookup(mock, true).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/pricing", now, 7, now, now))
	expectOverrideRates(mock).WillReturnRows(overrideRateRows().AddRow(1, 9, "input", "token", "", 1, 1_000_000))
	mock.ExpectRollback()
	called := false
	before, after, err := repository.ReplaceOverride(context.Background(), validReplaceCommand(2), func(existing *PriceOverride) error {
		called = true
		if existing == nil || existing.ID != 9 || existing.Version != 2 || len(existing.Rates) != 1 {
			t.Fatalf("locked existing = %#v", existing)
		}
		return wantErr
	})
	if before != nil || after != nil || !called || !errors.Is(err, wantErr) {
		t.Fatalf("validated replace = %#v, %#v, called=%v, err=%v", before, after, called, err)
	}
	assertAIModelPricingSQLMock(t, mock)
}

func TestAIModelPricingGormRepositoryRestoreDeletesVersionedHeadAndCascadesRates(t *testing.T) {
	repository, mock, closeDB := newAIModelPricingMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectOverrideLookup(mock, true).WillReturnRows(overrideRows().AddRow(9, "openai", "gpt-reviewed", 2, "https://openai.com/pricing", now, 7, now, now))
	expectOverrideRates(mock).WillReturnRows(overrideRateRows().AddRow(1, 9, "input", "token", "", 1, 1_000_000))
	mock.ExpectExec("DELETE FROM `ai_model_price_overrides` WHERE id = .* AND version =").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	before, err := repository.DeleteOverride(context.Background(), DeleteOverrideCommand{CatalogVendor: "openai", ModelID: "gpt-reviewed", ExpectedVersion: 2}, nil)
	if err != nil || before == nil || before.Version != 2 {
		t.Fatalf("DeleteOverride = %#v, %v", before, err)
	}
	assertAIModelPricingSQLMock(t, mock)
}

func newAIModelPricingMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	repository := NewGormRepository(&database.Client{Gorm: db, SQL: sqlDB})
	return repository, mock, func() { _ = sqlDB.Close() }
}

func expectOverrideLookup(mock sqlmock.Sqlmock, locked bool) *sqlmock.ExpectedQuery {
	query := regexp.QuoteMeta("SELECT * FROM `ai_model_price_overrides` WHERE catalog_vendor = ? AND model_id = ? ORDER BY id ASC LIMIT ?")
	if locked {
		query += " FOR UPDATE"
	}
	return mock.ExpectQuery(query).WithArgs("openai", "gpt-reviewed", 2)
}

func expectOverrideRates(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_model_price_override_rates` WHERE override_id = ? ORDER BY category ASC, unit ASC, tier_key ASC, id ASC")).WithArgs(uint64(9))
}

func overrideRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "catalog_vendor", "model_id", "version", "source_url", "verified_at", "updated_by", "created_at", "updated_at"})
}

func overrideRateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "override_id", "category", "unit", "tier_key", "price_units", "unit_scale"})
}

func validReplaceCommand(expectedVersion uint64) ReplaceOverrideCommand {
	return ReplaceOverrideCommand{
		CatalogVendor: "openai", ModelID: "gpt-reviewed", ExpectedVersion: expectedVersion,
		SourceURL: "https://openai.com/pricing", VerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), UpdatedBy: 7,
		Rates: []PriceOverrideRate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 125_000_000, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 350_000_000, UnitScale: 1_000_000},
		},
	}
}

func assertAIModelPricingSQLMock(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
