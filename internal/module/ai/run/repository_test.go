package airun

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestRunModelMapsLikedAt(t *testing.T) {
	parsed, err := schema.Parse(&Run{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Run schema: %v", err)
	}
	field := parsed.LookUpField("LikedAt")
	if field == nil {
		t.Fatal("Run must persist LikedAt")
	}
	if field.DBName != "liked_at" {
		t.Fatalf("LikedAt column mismatch: %q", field.DBName)
	}
	if field.FieldType != reflect.TypeOf((*time.Time)(nil)) {
		t.Fatalf("LikedAt type mismatch: %v", field.FieldType)
	}
}

func TestRunDetailRowMarksMessageSummariesIgnoredByGorm(t *testing.T) {
	_, err := schema.Parse(&RunDetailRow{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("message summaries are response-only fields and must not be parsed as gorm relations: %v", err)
	}
}

func TestStatsSelectsIntegerAverageDuration(t *testing.T) {
	summarySQL := statsSummarySelectSQL()
	groupedSQL := statsGroupedSelectSQL("DATE(r.created_at) as date")

	for name, sql := range map[string]string{
		"summary": sqlSummaryLower(summarySQL),
		"grouped": sqlSummaryLower(groupedSQL),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "avg_duration_ms") {
				t.Fatalf("average duration alias is required, sql=%s", sql)
			}
			if strings.Contains(sql, "coalesce(avg(r.duration_ms)") {
				t.Fatalf("average duration must not scan raw MySQL AVG decimal into int64, sql=%s", sql)
			}
			if !strings.Contains(sql, "cast(round(avg(r.duration_ms)) as signed)") {
				t.Fatalf("average duration must be rounded and cast before scanning into int64, sql=%s", sql)
			}
		})
	}
}

func TestRepositorySQLUsesAppAndEventSchema(t *testing.T) {
	summarySQL := sqlSummaryLower(statsSummarySelectSQL())
	groupedSQL := sqlSummaryLower(statsGroupedSelectSQL("r.agent_id as agent_id, COALESCE(a.name, '') as agent_name"))

	if !strings.Contains(summarySQL, "r.status in (?, ?, ?)") {
		t.Fatalf("summary must count failed, canceled and timeout as failed terminal runs, sql=%s", summarySQL)
	}
	if !strings.Contains(groupedSQL, "r.agent_id as agent_id") || !strings.Contains(groupedSQL, "agent_name") {
		t.Fatalf("grouped agent stats must expose agent_id/agent_name, sql=%s", groupedSQL)
	}
}

func TestBillingDetailUsesThreeBoundedQueries(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	repo := &GormRepository{db: db}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, held_units, actual_units, status FROM `ai_usage_charges` WHERE run_id = ? LIMIT ?")).WithArgs(int64(44), 1).WillReturnRows(
		sqlmock.NewRows([]string{"id", "held_units", "actual_units", "status"}).AddRow(9, 900, 250, "settled"),
	)
	mock.ExpectQuery("SELECT .* FROM ai_usage_charge_items i JOIN ai_usage_charges c ON c.id = i.charge_id JOIN ai_provider_attempts a ON a.id = i.attempt_id AND a.run_id = c.run_id WHERE c.run_id = \\? ORDER BY a.attempt_no ASC, i.id ASC").WithArgs(int64(44)).WillReturnRows(
		sqlmock.NewRows([]string{"attempt_id", "attempt_no", "attempt_state", "category", "tier_key", "quantity", "unit", "unit_price_units", "unit_scale", "amount_units"}).AddRow(101, 1, "succeeded", "input", "", 2, "token", 100, 1, 250),
	)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, attempt_no, state, provider_request_id, usage_status, usage_json FROM `ai_provider_attempts` WHERE run_id = ? ORDER BY attempt_no ASC")).WithArgs(int64(44)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "attempt_no", "state", "provider_request_id", "usage_status", "usage_json"}).AddRow(101, 1, "succeeded", "provider-1", "complete", `{"status":"complete","items":[]}`),
	)

	charge, items, attempts, err := repo.BillingDetail(context.Background(), 44)
	if err != nil {
		t.Fatalf("BillingDetail returned error: %v", err)
	}
	if charge == nil || charge.HeldUnits != 900 || len(items) != 1 || items[0].AttemptNo != 1 || len(attempts) != 1 || attempts[0].ProviderRequestID != "provider-1" {
		t.Fatalf("unexpected billing facts: charge=%#v items=%#v attempts=%#v", charge, items, attempts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("billing detail must use exactly the three expected set queries: %v", err)
	}
}

func sqlSummaryLower(sql string) string {
	return strings.ToLower(strings.Join(strings.Fields(sql), " "))
}
