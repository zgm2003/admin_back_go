package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"admin_back_go/internal/databaseevolution"
)

func TestQueryEvidenceIsExecutableAndOnlyAppliesAcceptedIndexes(t *testing.T) {
	root := backendRoot(t)
	manifestPath := filepath.Join(root, "database", "reconciliation", "040_query_candidates.json")
	candidates, err := databaseevolution.LoadQueryManifest(manifestPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 12 {
		t.Fatalf("query candidates=%d want 12", len(candidates))
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) != 12 {
		t.Fatalf("manifest JSON err=%v count=%d", err, len(raw))
	}

	script, err := os.ReadFile(filepath.Join(root, "scripts", "database", "capture-query-evidence.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	scriptBody := strings.ToLower(string(script))
	for _, required := range []string{
		"explain analyze format=tree", "performance_schema", "row_distribution_sql",
		"alter index", "invisible", "visible", "warm_run_count = 5", "max_rows_examined", "max_p95_ms",
		"accepted_indexes.json",
	} {
		if !strings.Contains(scriptBody, required) {
			t.Errorf("query evidence script missing %q", required)
		}
	}

	applySQL, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "041_apply_proven_indexes.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bdrop\s+(?:table|column)\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
	} {
		if forbidden.Match(applySQL) {
			t.Errorf("proven index SQL contains forbidden mutation %q", forbidden)
		}
	}
}
