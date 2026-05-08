package airun

import (
	"strings"
	"testing"
)

func TestStatsSelectsIntegerAverageLatency(t *testing.T) {
	summarySQL := statsSummarySelectSQL()
	groupedSQL := statsGroupedSelectSQL("DATE(r.created_at) as date")

	for name, sql := range map[string]string{
		"summary": sqlSummaryLower(summarySQL),
		"grouped": sqlSummaryLower(groupedSQL),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "avg_latency_ms") {
				t.Fatalf("average latency alias is required, sql=%s", sql)
			}
			if strings.Contains(sql, "coalesce(avg(r.latency_ms)") {
				t.Fatalf("average latency must not scan raw MySQL AVG decimal into int64, sql=%s", sql)
			}
			if !strings.Contains(sql, "cast(round(avg(r.latency_ms)) as signed)") {
				t.Fatalf("average latency must be rounded and cast before scanning into int64, sql=%s", sql)
			}
		})
	}
}

func sqlSummaryLower(sql string) string {
	return strings.ToLower(strings.Join(strings.Fields(sql), " "))
}
