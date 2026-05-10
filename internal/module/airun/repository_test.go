package airun

import (
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

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

func sqlSummaryLower(sql string) string {
	return strings.ToLower(strings.Join(strings.Fields(sql), " "))
}
