package aiprovider

import (
	"context"
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
