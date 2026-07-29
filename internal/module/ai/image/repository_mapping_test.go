package aiimage

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestListImageAgentsRequiresMappedProviderRoute(t *testing.T) {
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
	mock.ExpectQuery("JOIN ai_provider_models AS m ON .*m.status = \\? AND m.mapping_status = \\?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "avatar"}))

	if _, err := (&GormRepository{db: db}).ListImageAgents(context.Background(), "image_generate"); err != nil {
		t.Fatalf("ListImageAgents: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("image agent list did not enforce mapped route: %v", err)
	}
}
