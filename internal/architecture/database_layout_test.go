package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations(t *testing.T) {
	root := backendRoot(t)
	legacy, err := filepath.Glob(filepath.Join(root, "database", "legacy-migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob legacy migrations: %v", err)
	}
	if len(legacy) < 40 {
		t.Fatalf("legacy migrations=%d", len(legacy))
	}

	active, err := filepath.Glob(filepath.Join(root, "database", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob active migrations: %v", err)
	}
	if len(active) < 1 || filepath.Base(active[0]) != "202607150001_baseline.sql" {
		t.Fatalf("active migrations=%v", active)
	}
	versionedMigration := regexp.MustCompile(`^[0-9]{12}_[a-z0-9_]+\.sql$`)
	for index, path := range active {
		name := filepath.Base(path)
		if !versionedMigration.MatchString(name) {
			t.Fatalf("active migration has invalid name %q", name)
		}
		if index > 0 && name <= filepath.Base(active[index-1]) {
			t.Fatalf("active migrations are not strictly ordered: %v", active)
		}
	}

	data, err := os.ReadFile(filepath.Join(root, "scripts", "database", "atlas.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a") {
		t.Fatal("Atlas image is not digest pinned")
	}

	atlasSum, err := os.ReadFile(filepath.Join(root, "database", "migrations", "atlas.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(atlasSum), "h1:") || !strings.Contains(string(atlasSum), "202607150001_baseline.sql h1:") {
		t.Fatalf("Atlas checksum does not cover the baseline migration: %s", atlasSum)
	}
	for _, migration := range active {
		if !strings.Contains(string(atlasSum), filepath.Base(migration)+" h1:") {
			t.Fatalf("Atlas checksum does not cover %s", filepath.Base(migration))
		}
	}
	if _, err := os.Stat(filepath.Join(root, "atlas.sum")); !os.IsNotExist(err) {
		t.Fatal("Atlas checksum must have a single source in database/migrations")
	}

	attributes, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"/database/migrations/*.sql text eol=lf",
		"/database/migrations/atlas.sum text eol=lf",
	} {
		if !strings.Contains(string(attributes), rule) {
			t.Fatalf("missing LF guard %q", rule)
		}
	}
}

func TestAtlasHashGetsOnlyTheMigrationDirectoryWriteMount(t *testing.T) {
	root := backendRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "scripts", "database", "atlas.ps1"))
	if err != nil {
		t.Fatalf("read Atlas wrapper: %v", err)
	}
	body := string(script)
	for _, required := range []string{
		`"${root}:/workspace:ro"`,
		`$Command -eq 'migrate'`,
		`$Arguments.Count -gt 0`,
		`$Arguments[0] -eq 'hash'`,
		`Join-Path $root 'database\migrations'`,
		`"${migrations}:/workspace/database/migrations:rw"`,
		`@mounts`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Atlas wrapper missing scoped hash mount contract %q", required)
		}
	}
	if strings.Contains(body, `"${root}:/workspace:rw"`) {
		t.Fatal("Atlas wrapper must never grant the entire repository write access")
	}
	if strings.Count(body, ":rw") != 1 {
		t.Fatal("only migrate hash may receive one writable mount")
	}
}

func TestDatabaseVerificationPinsImmutableDockerInputs(t *testing.T) {
	root := backendRoot(t)
	scriptPath := filepath.Join(root, "scripts", "verify-database.ps1")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read shared database verifier: %v", err)
	}
	expandedVerifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-expanded-schema.ps1"))
	if err != nil {
		t.Fatalf("read expanded-schema verifier: %v", err)
	}
	reconcileVerifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "reconcile.ps1"))
	if err != nil {
		t.Fatalf("read reconciliation verifier: %v", err)
	}

	combined := string(script) + "\n" + string(expandedVerifier)
	verificationText := string(script) + "\n" + string(expandedVerifier) + "\n" + string(reconcileVerifier)
	for _, required := range []string{
		"mysql:8.4.10",
		"arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a",
		"[ValidateSet('all', 'empty', 'imported')]",
		"$Mode -in @('all', 'imported')",
		"'run', '--detach'",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("database verification is missing immutable input %q", required)
		}
	}

	for _, required := range []string{
		"all-nondestructive",
		"030_verify_schema.sql",
		"036_verify_export_cleanup.sql",
		"037_verify_cron_task_metadata.sql",
		"query-manifest",
		"git ls-files",
		"admin-go.env",
		".cnf",
		"schema_sha256",
		"--sql-mode=NO_ENGINE_SUBSTITUTION",
		"schema convergence diagnostic",
	} {
		if !strings.Contains(verificationText, required) {
			t.Fatalf("shared database verifier is missing gate %q", required)
		}
	}
	if !strings.Contains(string(script), `${Database}?parseTime`) {
		t.Fatal("database verifier must delimit the schema variable before DSN query parameters")
	}
	if !strings.Contains(string(script), "$primaryError") || strings.Contains(string(script), "-not $?") {
		t.Fatal("database verifier must preserve the primary failure independently from cleanup command status")
	}
}

func TestDatabaseVerificationUsesPostContractReconciliationSet(t *testing.T) {
	root := backendRoot(t)
	verifier, err := os.ReadFile(filepath.Join(root, "scripts", "verify-database.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-expanded-schema.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	reconcile, err := os.ReadFile(filepath.Join(root, "scripts", "database", "reconcile.ps1"))
	if err != nil {
		t.Fatal(err)
	}

	stage := regexp.MustCompile(`(?s)'post-contract'\s*=\s*@\((.*?)\)`).FindSubmatch(reconcile)
	if len(stage) != 2 {
		t.Fatal("reconcile.ps1 must define one post-contract stage")
	}
	for _, file := range []string{
		"001_ledger.sql",
		"010_expand_core.sql",
		"020_backfill_core.sql",
		"041_apply_proven_indexes.sql",
		"042_add_ai_image_soft_delete.sql",
		"043_register_export_cleanup.sql",
		"044_realtime_retention.sql",
		"045_repair_cron_task_utf8_metadata.sql",
	} {
		if !strings.Contains(string(stage[1]), file) {
			t.Fatalf("post-contract reconciliation is missing %s", file)
		}
	}
	for _, retiredSourceStage := range []string{"021_backfill_ai.sql", "046_retire_client_version_surface.sql"} {
		if strings.Contains(string(stage[1]), retiredSourceStage) {
			t.Fatalf("post-contract reconciliation must not replay %s after its source tables were contracted", retiredSourceStage)
		}
	}

	for _, required := range []string{
		"-Stage 'post-contract'",
		"reconciliationApplied -ne 8",
		"reconciliationSkipped -ne 8",
		"-PostContract",
	} {
		if !strings.Contains(string(verifier), required) {
			t.Fatalf("database verifier is missing post-contract replay guard %q", required)
		}
	}
	for _, required := range []string{
		"[switch]$PostContract",
		"051_verify_admin_rows.sql",
		"052_verify_ai_contract.sql",
		"053_verify_admin_only.sql",
	} {
		if !strings.Contains(string(expanded), required) {
			t.Fatalf("expanded verifier is missing post-contract invariant %q", required)
		}
	}
}

func TestDatabaseVerifierFixturePreservesCanonicalCheckConstraints(t *testing.T) {
	root := backendRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "scripts", "verify-database.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"ai_reply_commands", "ai_video_tasks", "authz_principal_versions", "schema_reconciliation_runs",
	} {
		pattern := regexp.MustCompile(`(?i)DROP\s+TABLE\s+` + "`" + regexp.QuoteMeta(table) + "`" + `\s*;`)
		if pattern.Match(script) {
			t.Fatalf("synthetic fixture recreates canonical CHECK constraint text for %s", table)
		}
	}
	if regexp.MustCompile("(?i)DROP\\s+(?:CHECK|CONSTRAINT)\\s+`chk_export_tasks_platform`").Match(script) {
		t.Fatal("synthetic fixture must not drop the canonical export platform CHECK")
	}

	lastPosition := -1
	for _, marker := range []string{
		"INSERT INTO `auth_platforms` (`id`,`code`,`name`,`login_types`,`created_at`,`updated_at`)",
		"ALTER TABLE `export_tasks` ALTER CHECK `chk_export_tasks_platform` NOT ENFORCED;",
		"VALUES (900001,900001,'','Synthetic export fixture','',1,2,UTC_TIMESTAMP(),UTC_TIMESTAMP());",
		"$firstRun = Invoke-Reconciliation",
		"ALTER TABLE `export_tasks` ALTER CHECK `chk_export_tasks_platform` ENFORCED;",
	} {
		position := strings.Index(string(script), marker)
		if position <= lastPosition {
			t.Fatalf("synthetic fixture must preserve CHECK objects while loading historical rows; missing or out-of-order %q", marker)
		}
		lastPosition = position
	}
}
