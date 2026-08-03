package aiprovider

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/officialmodel"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpdateModelMappingOnlyWritesMappingFacts(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	mappedAt := time.Date(2026, 7, 28, 9, 30, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE `ai_provider_models` SET `mapped_at`=\\?,`mapping_status`=\\?,`official_catalog_version`=\\?,`official_model_id`=\\? WHERE id = \\?").
		WithArgs(mappedAt, officialmodel.MappingStatusMapped, "official_models_v1", "gpt-4.1-mini", uint64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = (&GormRepository{db: db}).UpdateModelMapping(context.Background(), 9, officialmodel.IdentityMapping{
		OfficialModelID: "gpt-4.1-mini",
		CatalogVersion:  "official_models_v1",
		Status:          officialmodel.MappingStatusMapped,
		MappedAt:        &mappedAt,
	})
	if err != nil {
		t.Fatalf("UpdateModelMapping: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mapping update changed non-mapping columns: %v", err)
	}
}

func TestReconcileModelsPreservesExistingIDsInBothScopes(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		scope      ModelReconcileScope
		query      string
		queryArgs  []driver.Value
		existing   *sqlmock.Rows
		updateRuns int
	}{
		{
			name: "legacy chat only", scope: ModelReconcileChatOnly,
			query:      "SELECT .* FROM `ai_provider_models` WHERE provider_id = \\? AND model_kind = \\? ORDER BY id ASC FOR UPDATE",
			queryArgs:  []driver.Value{uint64(7), ModelKindChat},
			existing:   providerModelRows().AddRow(uint64(31), uint64(7), "gpt-5.6", ModelKindChat, "old", nil, nil, officialmodel.MappingStatusUnmapped, nil, 1, now, now),
			updateRuns: 1,
		},
		{
			name: "typed complete catalog", scope: ModelReconcileAll,
			query:     "SELECT .* FROM `ai_provider_models` WHERE provider_id = \\? ORDER BY id ASC FOR UPDATE",
			queryArgs: []driver.Value{uint64(7)},
			existing: providerModelRows().
				AddRow(uint64(31), uint64(7), "gpt-5.6", ModelKindChat, "old", nil, nil, officialmodel.MappingStatusUnmapped, nil, 1, now, now).
				AddRow(uint64(32), uint64(7), "embed-v1", ModelKindEmbedding, "embed", nil, nil, officialmodel.MappingStatusUnmapped, nil, 1, now, now),
			updateRuns: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
				DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
			})
			if err != nil {
				t.Fatal(err)
			}

			mock.ExpectBegin()
			mock.ExpectQuery(test.query).WithArgs(test.queryArgs...).WillReturnRows(test.existing)
			for range test.updateRuns {
				mock.ExpectExec("UPDATE `ai_provider_models` SET .* WHERE id = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()

			repository := &GormRepository{db: db}
			err = repository.ReconcileModels(context.Background(), 7, test.scope, []ProviderModel{{
				ModelID: "gpt-5.6", ModelKind: ModelKindChat, DisplayName: "new", MappingStatus: officialmodel.MappingStatusUnmapped, Status: 1,
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("reconcile deleted, inserted, or renumbered an existing identity: %v", err)
			}
		})
	}
}

func providerModelRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "provider_id", "model_id", "model_kind", "display_name", "official_model_id", "official_catalog_version", "mapping_status", "mapped_at", "status", "created_at", "updated_at",
	})
}
