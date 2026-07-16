package databaseevolution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewFingerprintDocumentIncludesCommitAndCanonicalSchemaHash(t *testing.T) {
	fingerprint := Fingerprint{
		ServerVersion: "8.4.10",
		SQLMode:       "NO_ENGINE_SUBSTITUTION",
		Schema:        "admin",
		Tables: []Table{
			{Name: "users", AutoIncrement: 901, Columns: []Column{{Ordinal: 2, Name: "email"}, {Ordinal: 1, Name: "id"}}},
			{Name: "roles", AutoIncrement: 20, Columns: []Column{{Ordinal: 1, Name: "id"}}},
		},
	}
	canonical, err := CanonicalJSON(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(canonical))

	document, err := NewFingerprintDocument("0123456789abcdef0123456789abcdef01234567", fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if document.GitCommit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("git_commit=%q", document.GitCommit)
	}
	if document.SchemaSHA256 != wantHash {
		t.Fatalf("schema_sha256=%q want %q", document.SchemaSHA256, wantHash)
	}
	if document.Tables[0].Name != "roles" {
		t.Fatalf("document fingerprint was not normalized: %+v", document.Tables)
	}
}

func TestNewFingerprintDocumentRejectsEmptyCommit(t *testing.T) {
	if _, err := NewFingerprintDocument(" \t", Fingerprint{Schema: "admin"}); err == nil {
		t.Fatal("expected empty commit to be rejected")
	}
}

func TestWriteFingerprintDocumentAtomicallyReplacesOutput(t *testing.T) {
	document, err := NewFingerprintDocument(
		"0123456789abcdef0123456789abcdef01234567",
		Fingerprint{ServerVersion: "8.4.10", Schema: "admin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "admin.json")
	if err := os.WriteFile(outputPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteFingerprintDocument(outputPath, document); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var got FingerprintDocument
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, data)
	}
	if got.SchemaSHA256 != document.SchemaSHA256 || got.Schema != "admin" {
		t.Fatalf("unexpected document: %+v", got)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(outputPath), ".admin.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain: %v", temporaryFiles)
	}
}

func TestWriteFingerprintDocumentCleansTemporaryFileAfterRenameFailure(t *testing.T) {
	document, err := NewFingerprintDocument(
		"0123456789abcdef0123456789abcdef01234567",
		Fingerprint{Schema: "admin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	outputPath := filepath.Join(parent, "admin.json")
	if err := os.Mkdir(outputPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := WriteFingerprintDocument(outputPath, document); err == nil {
		t.Fatal("expected rename onto a directory to fail")
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(parent, ".admin.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain: %v", temporaryFiles)
	}
}

func TestCanonicalJSONSortsAndExcludesVolatileValues(t *testing.T) {
	in := Fingerprint{
		ServerVersion: "8.4.10",
		SQLMode:       "NO_ENGINE_SUBSTITUTION",
		Schema:        "admin",
		Tables: []Table{
			{Name: "users", AutoIncrement: 901, Columns: []Column{{Ordinal: 2, Name: "email"}, {Ordinal: 1, Name: "id"}}},
			{Name: "roles", AutoIncrement: 20, Columns: []Column{{Ordinal: 1, Name: "id"}}},
		},
	}

	first, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || strings.Contains(string(first), "901") {
		t.Fatalf("non-canonical output: %s", first)
	}
	if strings.Index(string(first), "roles") > strings.Index(string(first), "users") {
		t.Fatalf("tables not sorted: %s", first)
	}
	if strings.Index(string(first), `"name":"id"`) > strings.Index(string(first), `"name":"email"`) {
		t.Fatalf("columns not sorted: %s", first)
	}
}

func TestCanonicalJSONNormalizesNilAndEmptyCollections(t *testing.T) {
	withNil := Fingerprint{Schema: "admin", Tables: []Table{{Name: "users"}}}
	withEmpty := Fingerprint{
		Schema:      "admin",
		Tables:      []Table{{Name: "users", Columns: []Column{}, Indexes: []Index{}}},
		ForeignKeys: []ForeignKey{},
		Checks:      []Check{},
		Triggers:    []Trigger{},
		Routines:    []Routine{},
		Events:      []Event{},
	}
	first, err := CanonicalJSON(withNil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalJSON(withEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("nil and empty collections differ:\n%s\n%s", first, second)
	}
}

func TestCanonicalJSONExcludesPhysicalSchemaName(t *testing.T) {
	admin := Fingerprint{
		ServerVersion: "8.4.10",
		Schema:        "admin",
		Tables:        []Table{{Name: "users"}},
		ForeignKeys: []ForeignKey{{
			Table: "users", Name: "fk_users_role", Column: "role_id",
			ReferencedSchema: "admin", ReferencedTable: "roles", ReferencedColumn: "id",
		}},
	}
	restored := admin
	restored.Schema = "admin_restore_0123456789ab"
	restored.ForeignKeys = append([]ForeignKey(nil), admin.ForeignKeys...)
	restored.ForeignKeys[0].ReferencedSchema = restored.Schema

	adminJSON, err := CanonicalJSON(admin)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, err := CanonicalJSON(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(adminJSON, restoredJSON) {
		t.Fatalf("physical schema name changed canonical schema JSON:\n%s\n%s", adminJSON, restoredJSON)
	}
	adminDocument, err := NewFingerprintDocument("0123456789abcdef0123456789abcdef01234567", admin)
	if err != nil {
		t.Fatal(err)
	}
	restoredDocument, err := NewFingerprintDocument("0123456789abcdef0123456789abcdef01234567", restored)
	if err != nil {
		t.Fatal(err)
	}
	if adminDocument.SchemaSHA256 != restoredDocument.SchemaSHA256 {
		t.Fatalf("schema hashes differ: %s != %s", adminDocument.SchemaSHA256, restoredDocument.SchemaSHA256)
	}
	if adminDocument.Schema != "admin" || restoredDocument.Schema != "admin_restore_0123456789ab" {
		t.Fatalf("documents lost physical schema names: %q %q", adminDocument.Schema, restoredDocument.Schema)
	}
}

func TestCanonicalJSONIncludesBehavioralSchemaMetadata(t *testing.T) {
	newFingerprint := func() Fingerprint {
		characterSet := "utf8mb4"
		collation := "utf8mb4_0900_ai_ci"
		parameterMode := "IN"
		parameterName := "input"
		triggerCharacterSet := "utf8mb4"
		eventCollation := "utf8mb4_0900_ai_ci"
		return Fingerprint{
			Schema: "admin",
			Tables: []Table{{
				Name:    "users",
				Columns: []Column{{Name: "email", CharacterSet: &characterSet, Collation: &collation}},
				Indexes: []Index{{Name: "idx_email", Visible: true}},
			}},
			ForeignKeys: []ForeignKey{{Name: "fk_role", ReferencedSchema: "admin", ReferencedTable: "roles"}},
			Checks:      []Check{{Name: "chk_status", Enforced: true}},
			Triggers:    []Trigger{{Name: "before_update", ActionOrder: 1, CharacterSet: &triggerCharacterSet}},
			Routines: []Routine{{
				Name: "normalize_email", Type: "FUNCTION",
				Parameters: []RoutineParameter{{Ordinal: 1, Mode: &parameterMode, Name: &parameterName, DTDIdentifier: "varchar(255)"}},
			}},
			Events: []Event{{Name: "cleanup", ConnectionCollation: &eventCollation}},
		}
	}
	baseline, err := CanonicalJSON(newFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Fingerprint)
	}{
		{name: "column collation", mutate: func(value *Fingerprint) { other := "utf8mb4_bin"; value.Tables[0].Columns[0].Collation = &other }},
		{name: "index visibility", mutate: func(value *Fingerprint) { value.Tables[0].Indexes[0].Visible = false }},
		{name: "external referenced schema", mutate: func(value *Fingerprint) { value.ForeignKeys[0].ReferencedSchema = "identity" }},
		{name: "check enforcement", mutate: func(value *Fingerprint) { value.Checks[0].Enforced = false }},
		{name: "trigger order", mutate: func(value *Fingerprint) { value.Triggers[0].ActionOrder = 2 }},
		{name: "trigger character set", mutate: func(value *Fingerprint) { other := "latin1"; value.Triggers[0].CharacterSet = &other }},
		{name: "routine parameter", mutate: func(value *Fingerprint) { value.Routines[0].Parameters[0].DTDIdentifier = "text" }},
		{name: "event collation", mutate: func(value *Fingerprint) { other := "utf8mb4_bin"; value.Events[0].ConnectionCollation = &other }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := newFingerprint()
			test.mutate(&changed)
			canonical, err := CanonicalJSON(changed)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(baseline, canonical) {
				t.Fatalf("%s did not affect canonical schema JSON", test.name)
			}
		})
	}
}

func TestValidateSchemaDSNRejectsWrongSchemaWithoutLeakingCredentials(t *testing.T) {
	const raw = "admin_user:super-secret@tcp(127.0.0.1:3306)/other?parseTime=true"
	err := ValidateSchemaDSN(raw, "admin")
	if err == nil {
		t.Fatal("expected schema mismatch")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), raw) {
		t.Fatalf("validation error leaked credentials: %v", err)
	}
}

func TestValidateSchemaDSNAcceptsMatchingSchema(t *testing.T) {
	const raw = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	if err := ValidateSchemaDSN(raw, "admin"); err != nil {
		t.Fatalf("ValidateSchemaDSN() error = %v", err)
	}
}

func TestTableChildQueriesUseTheBaseTableInventory(t *testing.T) {
	for name, query := range map[string]string{
		"columns": columnsQuery,
		"indexes": indexesQuery,
	} {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "information_schema.tables") || !strings.Contains(normalized, "table_type = 'base table'") {
			t.Fatalf("%s query is not restricted to base tables: %s", name, normalized)
		}
	}
}

func TestCaptureReadsStableInformationSchemaFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT @@version, @@sql_mode")).
		WillReturnRows(sqlmock.NewRows([]string{"version", "sql_mode"}).AddRow("8.4.10", "NO_ENGINE_SUBSTITUTION"))
	mock.ExpectQuery(`(?s)FROM information_schema\.tables`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "engine", "table_collation", "table_comment", "auto_increment"}).
			AddRow("users", "InnoDB", "utf8mb4_0900_ai_ci", "Users", 99))
	mock.ExpectQuery(`(?s)FROM information_schema\.columns`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "ordinal_position", "column_name", "column_type", "is_nullable", "column_default", "extra", "generation_expression", "column_comment", "character_set_name", "collation_name"}).
			AddRow("users", 2, "email", "varchar(255)", "YES", nil, "", "", "Email", "utf8mb4", "utf8mb4_0900_ai_ci").
			AddRow("users", 1, "id", "bigint", "NO", nil, "auto_increment", "", "", nil, nil))
	mock.ExpectQuery(`(?s)FROM information_schema\.statistics`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "index_name", "non_unique", "index_type", "seq_in_index", "column_name", "expression", "sub_part", "collation", "is_visible"}).
			AddRow("users", "PRIMARY", 0, "BTREE", 1, "id", nil, nil, "A", "YES"))
	mock.ExpectQuery(`(?s)FROM information_schema\.key_column_usage`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "constraint_name", "ordinal_position", "column_name", "referenced_table_schema", "referenced_table_name", "referenced_column_name", "update_rule", "delete_rule"}).
			AddRow("users", "fk_users_role", 1, "role_id", "admin", "roles", "id", "RESTRICT", "RESTRICT"))
	mock.ExpectQuery(`(?s)FROM information_schema\.table_constraints`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"table_name", "constraint_name", "check_clause", "enforced"}).
			AddRow("users", "chk_users_status", "(`status` in (1,2))", "YES"))
	mock.ExpectQuery(`(?s)FROM information_schema\.triggers`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"trigger_name", "event_object_table", "event_manipulation", "action_timing", "action_orientation", "action_order", "action_statement", "sql_mode", "character_set_client", "collation_connection", "database_collation"}).
			AddRow("users_before_update", "users", "UPDATE", "BEFORE", "ROW", 2, "SET NEW.updated_at = NOW()", "STRICT_TRANS_TABLES", "utf8mb4", "utf8mb4_0900_ai_ci", "utf8mb4_0900_ai_ci"))
	mock.ExpectQuery(`(?s)FROM information_schema\.routines`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"routine_name", "routine_type", "dtd_identifier", "routine_definition", "sql_data_access", "is_deterministic", "security_type", "sql_mode", "routine_comment", "character_set_client", "collation_connection", "database_collation"}).
			AddRow("normalize_email", "FUNCTION", "varchar(255)", "RETURN LOWER(input)", "READS SQL DATA", "YES", "DEFINER", "STRICT_TRANS_TABLES", "", nil, nil, "utf8mb4_0900_ai_ci"))
	mock.ExpectQuery(`(?s)FROM information_schema\.parameters`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"specific_name", "routine_type", "ordinal_position", "parameter_mode", "parameter_name", "dtd_identifier", "character_set_name", "collation_name"}).
			AddRow("normalize_email", "FUNCTION", 0, nil, nil, "varchar(255)", "utf8mb4", "utf8mb4_0900_ai_ci").
			AddRow("normalize_email", "FUNCTION", 1, "IN", "input", "varchar(255)", "utf8mb4", "utf8mb4_0900_ai_ci"))
	mock.ExpectQuery(`(?s)FROM information_schema\.events`).WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"event_name", "event_definition", "event_type", "execute_at", "interval_value", "interval_field", "starts", "ends", "status", "on_completion", "sql_mode", "event_comment", "time_zone", "character_set_client", "collation_connection", "database_collation"}).
			AddRow("cleanup_sessions", "DELETE FROM user_sessions WHERE revoked_at IS NOT NULL", "RECURRING", nil, "1", "DAY", "2026-07-16 00:00:00.000000", nil, "ENABLED", "NOT PRESERVE", "STRICT_TRANS_TABLES", "", "SYSTEM", "utf8mb4", nil, "utf8mb4_0900_ai_ci"))

	got, err := Capture(context.Background(), db, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerVersion != "8.4.10" || got.SQLMode != "NO_ENGINE_SUBSTITUTION" || got.Schema != "admin" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if len(got.Tables) != 1 || len(got.Tables[0].Columns) != 2 || len(got.Tables[0].Indexes) != 1 {
		t.Fatalf("unexpected tables: %+v", got.Tables)
	}
	if got.Tables[0].Columns[1].CharacterSet == nil || *got.Tables[0].Columns[1].CharacterSet != "utf8mb4" || got.Tables[0].Columns[0].CharacterSet != nil {
		t.Fatalf("unexpected column character sets: %+v", got.Tables[0].Columns)
	}
	if !got.Tables[0].Indexes[0].Visible {
		t.Fatalf("primary index was not visible: %+v", got.Tables[0].Indexes[0])
	}
	if len(got.Checks) != 1 || got.Checks[0].Name != "chk_users_status" {
		t.Fatalf("unexpected checks: %+v", got.Checks)
	}
	if len(got.ForeignKeys) != 1 || got.ForeignKeys[0].ReferencedSchema != "admin" {
		t.Fatalf("unexpected foreign keys: %+v", got.ForeignKeys)
	}
	if !got.Checks[0].Enforced {
		t.Fatalf("check enforcement was not captured: %+v", got.Checks[0])
	}
	if len(got.Triggers) != 1 || got.Triggers[0].ActionOrder != 2 {
		t.Fatalf("unexpected triggers: %+v", got.Triggers)
	}
	if got.Triggers[0].CharacterSet == nil || *got.Triggers[0].CharacterSet != "utf8mb4" || got.Triggers[0].DatabaseCollation == nil {
		t.Fatalf("trigger creation context was not captured: %+v", got.Triggers[0])
	}
	if len(got.Routines) != 1 || len(got.Routines[0].Parameters) != 2 || got.Routines[0].Parameters[1].Name == nil || *got.Routines[0].Parameters[1].Name != "input" {
		t.Fatalf("unexpected routines: %+v", got.Routines)
	}
	if got.Routines[0].CharacterSet != nil || got.Routines[0].ConnectionCollation != nil || got.Routines[0].DatabaseCollation == nil {
		t.Fatalf("routine nullable metadata was not preserved: %+v", got.Routines[0])
	}
	if len(got.Events) != 1 || got.Events[0].ExecuteAt != nil || got.Events[0].Starts == nil || *got.Events[0].Starts != "2026-07-16 00:00:00.000000" {
		t.Fatalf("unexpected events: %+v", got.Events)
	}
	if got.Events[0].CharacterSet == nil || got.Events[0].ConnectionCollation != nil || got.Events[0].DatabaseCollation == nil {
		t.Fatalf("event creation context was not preserved: %+v", got.Events[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
