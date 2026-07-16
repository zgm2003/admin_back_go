package databaseevolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadQueryManifestValidatesExecutableCandidates(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "internal", "module", "auth", "session.go")
	if err := os.MkdirAll(filepath.Dir(repositoryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repositoryPath, []byte("package auth\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "database", "reconciliation", "040_query_candidates.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `[{"name":"session_by_user","repository_file":"internal/module/auth/session.go","sql":"SELECT id FROM user_sessions WHERE user_id=:user_id AND platform=:platform ORDER BY refresh_expires_at DESC, id DESC LIMIT :limit","bindings":{"user_id":1,"platform":"admin","limit":20},"expected_order":["refresh_expires_at DESC","id DESC"],"row_distribution_sql":"SELECT platform,COUNT(*) rows_count FROM user_sessions GROUP BY platform","proposed_index":"CREATE INDEX idx_user_sessions_user_platform_active_refresh ON user_sessions (user_id,platform,is_del,revoked_at,refresh_expires_at,id)","max_rows_examined":20,"max_p95_ms":100}]`
	if err := os.WriteFile(manifestPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	candidates, err := LoadQueryManifest(manifestPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Name != "session_by_user" || candidates[0].RepositoryFile != "internal/module/auth/session.go" {
		t.Fatalf("candidates=%+v", candidates)
	}
}

func TestLoadQueryManifestRejectsUnsafeOrNonDeterministicCandidates(t *testing.T) {
	root := t.TempDir()
	base := QueryCandidate{
		Name: "candidate", RepositoryFile: "internal/module/auth/session.go",
		SQL:           "SELECT id FROM user_sessions WHERE user_id=:user_id ORDER BY id DESC LIMIT :limit",
		Bindings:      map[string]any{"user_id": float64(1), "limit": float64(20)},
		ExpectedOrder: []string{"id DESC"}, RowDistributionSQL: "SELECT platform,COUNT(*) rows_count FROM user_sessions GROUP BY platform",
		ProposedIndex: "CREATE INDEX idx_candidate ON user_sessions (user_id,id)", MaxRowsExamined: 20, MaxP95MS: 100,
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "module", "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "module", "auth", "session.go"), []byte("package auth\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*QueryCandidate)
		want   string
	}{
		{name: "select star", mutate: func(value *QueryCandidate) { value.SQL = "SELECT * FROM user_sessions ORDER BY id DESC LIMIT :limit" }, want: "SELECT *"},
		{name: "missing id order", mutate: func(value *QueryCandidate) {
			value.SQL = "SELECT id FROM user_sessions WHERE user_id=:user_id ORDER BY created_at DESC LIMIT :limit"
			value.ExpectedOrder = []string{"created_at DESC"}
		}, want: "id tie-breaker"},
		{name: "empty bindings", mutate: func(value *QueryCandidate) { value.Bindings = nil }, want: "bindings"},
		{name: "unsafe ddl", mutate: func(value *QueryCandidate) { value.ProposedIndex = "DROP TABLE users" }, want: "CREATE INDEX"},
		{name: "outside module", mutate: func(value *QueryCandidate) { value.RepositoryFile = "../secret.go" }, want: "repository_file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.Bindings = map[string]any{"user_id": float64(1), "limit": float64(20)}
			candidate.ExpectedOrder = append([]string(nil), base.ExpectedOrder...)
			test.mutate(&candidate)
			err := ValidateQueryCandidates([]QueryCandidate{candidate}, root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want %q", err, test.want)
			}
		})
	}
}
