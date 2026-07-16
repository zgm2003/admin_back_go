package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCoreBackfillUsesOnlyEvidenceBackedMappings(t *testing.T) {
	path := filepath.Join(backendRoot(t), "database", "reconciliation", "020_backfill_core.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(data))
	for _, required := range []string{
		"platform` = 'admin'", "kind` = 'user_list'", "bucket_domain",
		"authz_principal_versions", "auth.verify_code.ttl_minutes",
		"total_consume_cents", "signal sqlstate '45000'", "source_task_id",
		"candidate_count = 1",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("core backfill missing evidence rule %q", required)
		}
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bdrop\s+table\b`),
		regexp.MustCompile(`(?i)\bdrop\s+column\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
		regexp.MustCompile(`(?i)\btenant\b`),
	} {
		if forbidden.Match(data) {
			t.Errorf("core backfill contains forbidden SQL %q", forbidden)
		}
	}
}
