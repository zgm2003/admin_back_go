package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDatabaseBaselineSchemaContract(t *testing.T) {
	root := backendRoot(t)
	schemaPath := filepath.Join(root, "database", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read database/schema.sql: %v", err)
	}

	body := string(schema)
	normalized := strings.ToLower(body)
	for _, required := range []string{
		"create table `users`",
		"create table `ai_context_plans`",
		"create table `payment_orders`",
		"create table `schema_migrations`",
		"constraint `fk_mail_log_verification_codes_mail_log`",
		"constraint `chk_ai_runs_status`",
		"unique key `uk_wallet_transaction_source`",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("canonical schema missing %q", required)
		}
	}

	if count := strings.Count(normalized, "create table `"); count != 77 {
		t.Errorf("canonical schema table count=%d want 77", count)
	}
	if count := strings.Count(normalized, "create table `schema_migrations`"); count != 1 {
		t.Errorf("schema_migrations table count=%d want 1", count)
	}

	for _, forbidden := range []string{
		"atlas_schema_revisions",
		"schema_reconciliation_runs",
		"ai_billing_migration_metadata",
		"definer=",
		"insert into",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("canonical schema contains forbidden %q", forbidden)
		}
	}
	if regexp.MustCompile(`(?i)\bauto_increment\s*=\s*[0-9]+`).Match(schema) {
		t.Error("canonical schema contains a volatile auto-increment counter")
	}
	if !regexp.MustCompile(`(?i)engine=innodb`).Match(schema) {
		t.Error("canonical schema contains no InnoDB table")
	}
	if !regexp.MustCompile(`(?i)(default charset|character set)\s*=\s*utf8mb4`).Match(schema) {
		t.Error("canonical schema contains no utf8mb4 table")
	}
}

func TestCanonicalDatabaseBaselineIsPinnedAndConvergent(t *testing.T) {
	root := backendRoot(t)
	schema, err := os.ReadFile(filepath.Join(root, "database", "schema", "admin.hcl"))
	if err != nil {
		t.Fatalf("read canonical admin schema: %v", err)
	}
	body := string(schema)
	for _, required := range []string{
		`schema "admin"`, `table "users"`, `table "ai_image_files"`,
		`table "schema_reconciliation_runs"`, `table "atlas_schema_revisions"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("canonical schema missing %q", required)
		}
	}
	if regexp.MustCompile(`(?mi)^\s*auto_increment\s*=\s*[0-9]+\s*$`).Match(schema) {
		t.Fatal("canonical schema contains a volatile auto-increment counter")
	}
	if regexp.MustCompile(`(?i)\bdefiner\b`).Match(schema) {
		t.Fatal("canonical schema contains a definer")
	}

	establish := mustReadBaselineScript(t, root, "establish-baseline.ps1")
	for _, required := range []string{
		"atlas-runtime-common.ps1", "admin:atlas:migrate", "lock-run", "migrate", "set", "202607150001",
		"'migrate', 'validate'", "schema", "inspect", "ExpectedFingerprint", "Move-Item",
	} {
		if !strings.Contains(establish, required) {
			t.Fatalf("establish baseline script missing %q", required)
		}
	}
	runtimeCommon := mustReadBaselineScript(t, root, "atlas-runtime-common.ps1")
	if !strings.Contains(runtimeCommon, "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a") {
		t.Fatal("Atlas runtime helper is not digest pinned")
	}

	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, required := range []string{
		"^admin_(empty|imported)_[0-9a-f]{12}$", "schema", "apply", "--auto-approve",
		"admin.hcl", "fingerprint", "schema_sha256",
	} {
		if !strings.Contains(drift, required) {
			t.Fatalf("drift checker missing %q", required)
		}
	}
}

func TestDatabaseBaselineIncludesMailVerificationDiagnostics(t *testing.T) {
	root := backendRoot(t)
	migrationPath := filepath.Join(root, "database", "migrations", "202607230101_mail_verification_code_diagnostics.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read mail verification diagnostic migration: %v", err)
	}
	exactMigration := `CREATE TABLE ` + "`mail_log_verification_codes`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`mail_log_id`" + ` BIGINT UNSIGNED NOT NULL,
  ` + "`key_id`" + ` VARCHAR(64) NOT NULL,
  ` + "`code_enc`" + ` VARCHAR(255) NOT NULL,
  ` + "`expires_at`" + ` DATETIME NOT NULL,
  ` + "`created_at`" + ` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`uk_mail_log_verification_codes_mail_log`" + ` (` + "`mail_log_id`" + `),
  KEY ` + "`idx_mail_log_verification_codes_key_id_id`" + ` (` + "`key_id`" + `, ` + "`id`" + `),
  CONSTRAINT ` + "`fk_mail_log_verification_codes_mail_log`" + `
    FOREIGN KEY (` + "`mail_log_id`" + `) REFERENCES ` + "`mail_logs`" + ` (` + "`id`" + `)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO ` + "`permissions`" + `
(` + "`id`,`name`,`path`,`icon`,`parent_id`,`component`,`platform`,`type`,`sort`,`code`,`i18n_key`,`show_menu`,`status`,`is_del`" + `)
VALUES (515,'查看邮件日志及验证码','','',506,NULL,'admin',3,9,'system_mail_logView','',2,1,2);`
	if got := strings.TrimSpace(strings.ReplaceAll(string(migration), "\r\n", "\n")); got != exactMigration {
		t.Fatalf("mail verification diagnostic migration must match the approved SQL exactly\ngot:\n%s", got)
	}

	normalizedMigration := strings.ToLower(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(string(migration)))
	for _, statement := range []string{
		"insert into role_permissions", "update role_permissions", "delete from role_permissions",
		"replace into role_permissions", "truncate table role_permissions",
	} {
		if strings.Contains(normalizedMigration, statement) {
			t.Fatalf("mail verification diagnostic migration contains forbidden write %q", statement)
		}
	}

	schema, err := os.ReadFile(filepath.Join(root, "database", "schema", "admin.hcl"))
	if err != nil {
		t.Fatalf("read canonical admin schema: %v", err)
	}
	table := hclTableBlock(t, strings.ReplaceAll(string(schema), "\r\n", "\n"), "mail_log_verification_codes")
	for _, required := range []string{
		`schema = schema.admin`, `column "id"`, `type           = bigint`, `unsigned       = true`, `auto_increment = true`,
		`column "mail_log_id"`, `column "key_id"`, `type = varchar(64)`, `column "code_enc"`, `type = varchar(255)`,
		`column "expires_at"`, `column "created_at"`, `default = sql("CURRENT_TIMESTAMP")`,
		`columns = [column.id]`, `index "uk_mail_log_verification_codes_mail_log"`, `unique  = true`,
		`columns = [column.mail_log_id]`, `index "idx_mail_log_verification_codes_key_id_id"`,
		`columns = [column.key_id, column.id]`, `foreign_key "fk_mail_log_verification_codes_mail_log"`,
		`columns     = [column.mail_log_id]`, `ref_columns = [table.mail_logs.column.id]`,
		`on_update   = RESTRICT`, `on_delete   = RESTRICT`,
	} {
		if !strings.Contains(table, required) {
			t.Errorf("canonical mail verification diagnostic table missing %q", required)
		}
	}
	columns := regexp.MustCompile(`(?m)^  column "([^"]+)" \{$`).FindAllStringSubmatch(table, -1)
	wantColumns := []string{"id", "mail_log_id", "key_id", "code_enc", "expires_at", "created_at"}
	if len(columns) != len(wantColumns) {
		t.Fatalf("mail verification diagnostic columns=%d want %d: %v", len(columns), len(wantColumns), columns)
	}
	for index, want := range wantColumns {
		if columns[index][1] != want {
			t.Fatalf("mail verification diagnostic column %d=%q want %q", index, columns[index][1], want)
		}
	}
	for _, forbidden := range []string{`column "status"`, `column "updated_at"`, `column "is_del"`} {
		if strings.Contains(table, forbidden) {
			t.Fatalf("canonical mail verification diagnostic table contains forbidden %q", forbidden)
		}
	}

	verifySchema, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "030_verify_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	verifyRelations, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "031_verify_relations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"mail_verification_diagnostic_table", "mail_verification_diagnostic_columns",
		"mail_verification_diagnostic_column_shapes", "mail_verification_diagnostic_indexes",
		"mail_verification_diagnostic_foreign_key", "mail_log_verification_codes",
		"uk_mail_log_verification_codes_mail_log", "idx_mail_log_verification_codes_key_id_id",
		"fk_mail_log_verification_codes_mail_log", "on_update", "on_delete", "restrict",
	} {
		if !strings.Contains(strings.ToLower(string(verifySchema)), required) {
			t.Errorf("schema reconciliation missing mail diagnostic invariant %q", required)
		}
	}
	for _, required := range []string{
		"mail_verification_diagnostic_orphans", "mail_log_verification_codes", "mail_logs", "mail_log_id",
	} {
		if !strings.Contains(strings.ToLower(string(verifyRelations)), required) {
			t.Errorf("relation reconciliation missing mail diagnostic invariant %q", required)
		}
	}

	seed, err := os.ReadFile(filepath.Join(root, "database", "seeds", "admin_permissions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	permissionTuple := "(515, '查看邮件日志及验证码', '', '', 506, NULL, 'admin', 3, 9, 'system_mail_logView', '', 2, 1, 2)"
	if strings.Count(string(seed), permissionTuple) != 1 || !strings.Contains(string(migration), "VALUES (515,'查看邮件日志及验证码','','',506,NULL,'admin',3,9,'system_mail_logView','',2,1,2);") {
		t.Fatal("mail diagnostic permission 515 must have migration/seed parity")
	}
}

func TestMailVerificationDiagnosticDocumentation(t *testing.T) {
	root := backendRoot(t)
	read := func(path ...string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return strings.ToLower(string(body))
	}

	seedReadme := read("database", "seeds", "README.md")
	for _, required := range []string{"138", "system_mail_logview", "payment_redeem_code_list", "does not assign permissions to any role"} {
		if !strings.Contains(seedReadme, required) {
			t.Errorf("permission seed documentation missing %q", required)
		}
	}

	rotation := read("docs", "runbooks", "session-secret-rotation.md")
	for _, required := range []string{
		"drain", "stop", "old-current", "app_secret_previous", "mail diagnostic", "rekey",
		"zero previous", "zero unknown", "before", "api", "worker", "evidence", "backup", "secret generation",
	} {
		if !strings.Contains(rotation, required) {
			t.Errorf("session-secret rotation runbook missing %q", required)
		}
	}
	rekeyCommand := "go run ./cmd/admin-db mail-diagnostic-rekey"
	if strings.Count(rotation, rekeyCommand) != 2 {
		t.Fatalf("session-secret rotation runbook must require the mail diagnostic rekey command twice")
	}
	firstRekey := strings.Index(rotation, rekeyCommand)
	startGate := strings.Index(rotation, "before any new-current api or worker starts")
	if firstRekey < 0 || startGate < firstRekey {
		t.Fatal("initial mail diagnostic rekey must precede the new-current API/Worker start gate")
	}
	removeGate := strings.Index(rotation, "immediately before removing `app_secret_previous`")
	secondRekey := strings.LastIndex(rotation, rekeyCommand)
	if removeGate < 0 || secondRekey < removeGate || !strings.Contains(rotation[secondRekey:], "zero previous") || !strings.Contains(rotation[secondRekey:], "zero unknown") {
		t.Fatal("removing APP_SECRET_PREVIOUS must have an independent final mail diagnostic zero-reference rekey gate")
	}

	architecture := read("docs", "architecture.md")
	for _, required := range []string{
		"mail diagnostic", "owns", "plaintext", "audit", "tls", "app_secret_previous",
	} {
		if !strings.Contains(architecture, required) {
			t.Errorf("architecture documentation missing mail diagnostic boundary %q", required)
		}
	}

	envExample := read("deploy", "docker-first", "admin-go.env.example")
	for _, required := range []string{"jwt signing", "refresh-token pepper", "secretbox", "mail diagnostic"} {
		if !strings.Contains(envExample, required) {
			t.Errorf("Docker env example missing live APP_SECRET derivation %q", required)
		}
	}
	if strings.Contains(envExample, "session-cache") {
		t.Fatal("Docker env example must not claim an unused session-cache cryptographic derivation")
	}
	if strings.Contains(architecture, "session-cache key") || strings.Contains(architecture, "session-cache keys") {
		t.Fatal("architecture documentation must not claim an unused session-cache cryptographic derivation")
	}
}

func hclTableBlock(t *testing.T, schema string, name string) string {
	t.Helper()
	marker := `table "` + name + `" {`
	start := strings.Index(schema, marker)
	if start < 0 {
		t.Fatalf("canonical schema missing %s", marker)
	}
	depth := 0
	for index := start; index < len(schema); index++ {
		switch schema[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return schema[start : index+1]
			}
		}
	}
	t.Fatalf("canonical schema table %q has unbalanced braces", name)
	return ""
}

func TestEstablishBaselineValidatesHCLWithoutAtlasDevDatabase(t *testing.T) {
	root := backendRoot(t)
	establish := mustReadBaselineScript(t, root, "establish-baseline.ps1")
	if !strings.Contains(establish, "'schema', 'fmt'") {
		t.Fatal("establish baseline must validate inspected HCL with offline Atlas schema fmt")
	}
	if strings.Contains(establish, "'schema', 'inspect', '--url', 'file:///runtime/admin.hcl'") {
		t.Fatal("file schema inspect requires a dev database and cannot be used as offline HCL validation")
	}
}

func TestDriftCheckUsesOnlyAtlasOSSFeatures(t *testing.T) {
	root := backendRoot(t)
	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, proOnly := range []string{"--lock-name", "--lock-timeout"} {
		if strings.Contains(drift, proOnly) {
			t.Fatalf("drift checker uses Atlas Pro-only flag %q", proOnly)
		}
	}
}

func TestDriftCheckRebindsCanonicalSchemaToDisposableDatabase(t *testing.T) {
	root := backendRoot(t)
	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, required := range []string{
		`schema "admin"`, `schema.$emptyDatabase`, `file:///runtime/admin.hcl`,
	} {
		if !strings.Contains(drift, required) {
			t.Fatalf("drift checker does not rebind canonical schema: missing %q", required)
		}
	}
	if strings.Contains(drift, "--to', 'file:///workspace/database/schema/admin.hcl") {
		t.Fatal("drift checker applies canonical admin schema without rebinding its database name")
	}
	if strings.Contains(drift, ".Contains('schema.admin')") {
		t.Fatal("prefix matching misclassifies schema.admin_empty_* as an unrebound admin reference")
	}
	if !strings.Contains(drift, `\bschema\.admin\b`) {
		t.Fatal("drift checker must detect only an exact unrebound schema.admin reference")
	}
}

func mustReadBaselineScript(t *testing.T, root string, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "scripts", "database", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}
