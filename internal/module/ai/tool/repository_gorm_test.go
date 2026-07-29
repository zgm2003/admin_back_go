package aitool

import (
	"context"
	"testing"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormRepositoryListRuntimeToolsUsesActiveBindingAndToolPredicates(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewGormRepository(&database.Client{Gorm: db, SQL: sqlDB})
	mock.ExpectQuery("SELECT .* FROM ai_agent_tools at JOIN ai_tools t ON t.id = at.tool_id AND t.is_del = \\? WHERE at.agent_id = \\? AND at.status = \\? AND t.status = \\? AND t.risk_level = \\? ORDER BY at.id ASC").
		WithArgs(enum.CommonNo, uint64(3), enum.CommonYes, enum.CommonYes, RiskLow).
		WillReturnRows(sqlmock.NewRows([]string{
			"tool_id", "name", "code", "description", "parameters_json", "result_schema_json",
			"risk_level", "timeout_ms", "tool_status", "binding_status",
		}).AddRow(5, "查询用户量", "admin_user_count", "查询用户数量", `{"type":"object"}`, `{"type":"object"}`, RiskLow, 3000, enum.CommonYes, enum.CommonYes))

	rows, err := repository.ListRuntimeTools(context.Background(), 3)
	if err != nil || len(rows) != 1 || rows[0].ToolID != 5 {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
