package architecture

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"admin_back_go/internal/databaseevolution"
)

type dashboardIndexSpec struct {
	name    string
	table   string
	columns []string
}

var dashboardIndexCandidates = map[string]dashboardIndexSpec{
	"ai_run_dashboard_model_created": {
		name: "idx_ai_runs_model_created", table: "ai_runs", columns: []string{"model_id", "created_at", "id"},
	},
	"ai_run_dashboard_platform_created": {
		name: "idx_ai_runs_platform_created", table: "ai_runs", columns: []string{"platform", "created_at", "id"},
	},
	"ai_run_dashboard_billing_created": {
		name: "idx_ai_runs_billing_created", table: "ai_runs", columns: []string{"billing_status", "billing_reason", "created_at", "id"},
	},
	"ai_run_dashboard_attempt_error": {
		name: "idx_ai_provider_attempts_error_run", table: "ai_provider_attempts", columns: []string{"error_code", "run_id", "id"},
	},
}

// Populate this set only from the disposable database's accepted_indexes.json.
var evidenceAcceptedDashboardIndexes = map[string]struct{}{}

func TestAIRunDashboardSchema(t *testing.T) {
	root := backendRoot(t)
	manifestPath := filepath.Join(root, "database", "reconciliation", "20260729_ai_run_dashboard_query_candidates.json")
	candidates, err := databaseevolution.LoadQueryManifest(manifestPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != len(dashboardIndexCandidates) {
		t.Fatalf("dashboard query candidates=%d want %d", len(candidates), len(dashboardIndexCandidates))
	}
	if files := databaseevolution.QueryManifestFiles(candidates); !slices.Equal(files, []string{"internal/module/ai/run/dashboard_repository.go"}) {
		t.Fatalf("dashboard query files=%v", files)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		spec, exists := dashboardIndexCandidates[candidate.Name]
		if !exists {
			t.Errorf("unexpected dashboard query candidate %q", candidate.Name)
			continue
		}
		seen[candidate.Name] = struct{}{}
		wantDDL := "CREATE INDEX " + spec.name + " ON " + spec.table + " (" + strings.Join(spec.columns, ",") + ")"
		if candidate.ProposedIndex != wantDDL {
			t.Errorf("candidate %q proposed_index=%q want %q", candidate.Name, candidate.ProposedIndex, wantDDL)
		}
	}
	for name := range dashboardIndexCandidates {
		if _, exists := seen[name]; !exists {
			t.Errorf("dashboard query candidate %q is missing", name)
		}
	}

	hcl := normalizeDashboardSchemaText(readOfficialModelSchemaFile(t, filepath.Join(root, "database", "schema", "admin.hcl")))
	migrationPath := filepath.Join(root, "database", "migrations", "202607290101_ai_run_dashboard_indexes.sql")
	migrationBytes, migrationErr := os.ReadFile(migrationPath)
	if migrationErr != nil && !os.IsNotExist(migrationErr) {
		t.Fatal(migrationErr)
	}
	if len(evidenceAcceptedDashboardIndexes) == 0 && migrationErr == nil {
		t.Fatal("dashboard index migration must not exist before any candidate is accepted")
	}
	migration := strings.ToLower(string(migrationBytes))

	for candidateName, spec := range dashboardIndexCandidates {
		_, accepted := evidenceAcceptedDashboardIndexes[candidateName]
		indexMarker := `index "` + spec.name + `"`
		migrationMarker := "create index `" + spec.name + "`"
		if accepted {
			if migrationErr != nil {
				t.Fatalf("accepted index %q is missing migration", spec.name)
			}
			columns := make([]string, 0, len(spec.columns))
			for _, column := range spec.columns {
				columns = append(columns, "column."+column)
			}
			if !strings.Contains(hcl, indexMarker) || !strings.Contains(hcl, "columns = ["+strings.Join(columns, ", ")+"]") {
				t.Errorf("accepted index %q is missing from HCL with ordered columns %v", spec.name, spec.columns)
			}
			if !strings.Contains(migration, migrationMarker) {
				t.Errorf("accepted index %q is missing from migration", spec.name)
			}
			continue
		}
		if strings.Contains(hcl, indexMarker) || strings.Contains(migration, migrationMarker) {
			t.Errorf("unaccepted index %q must not be present", spec.name)
		}
	}
}

func normalizeDashboardSchemaText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(value, "\r\n", "\n"))), " ")
}
