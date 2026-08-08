package aiimage

import (
	"context"
	"testing"

	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestListImageAgentsRequiresMappedImageProviderRoute(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	mock.ExpectQuery("JOIN ai_provider_models AS m ON .*m.model_kind = \\? AND m.status = \\? AND m.mapping_status = \\?").
		WithArgs(enum.CommonNo, enum.CommonYes, aiprovider.ModelKindImage, enum.CommonYes, officialmodel.MappingStatusMapped, enum.CommonNo, enum.CommonYes, "image_generate").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "avatar"}))

	if _, err := (&GormRepository{db: db}).ListImageAgents(context.Background(), "image_generate"); err != nil {
		t.Fatalf("ListImageAgents: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("image agent list did not enforce mapped route: %v", err)
	}
}

func TestLoadAgentRuntimeRequiresMappedImageProviderRoute(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("JOIN ai_provider_models AS m ON .*m.model_kind = \\? AND m.status = \\? AND m.mapping_status = \\?").
		WithArgs(enum.CommonNo, aiprovider.ModelKindImage, enum.CommonYes, officialmodel.MappingStatusMapped, uint64(5), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "model_id"}).AddRow(uint64(5), "gpt-5.6"))

	runtime, err := (&GormRepository{db: db}).LoadAgentRuntime(context.Background(), 5)
	if err != nil || runtime == nil || runtime.AgentID != 5 {
		t.Fatalf("runtime=%#v err=%v", runtime, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("image runtime did not pin image model: %v", err)
	}
}
