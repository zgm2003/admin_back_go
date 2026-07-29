package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAIRunLatencyTimelineSchemaHasCanonicalColumns(t *testing.T) {
	root := backendRoot(t)
	migration := normalizeOfficialModelSchema(t, filepath.Join(root, "database", "migrations", "202607280102_ai_run_latency_timeline.sql"))
	for _, required := range []string{
		"add column request_received_at datetime(6) null",
		"add column accepted_at datetime(6) null",
		"add column claimed_at datetime(6) null",
		"add column claim_source varchar(16) not null default ''",
		"check (claim_source in ('','wake','poll','recovery'))",
		"add column prepare_started_at datetime(6) null",
		"add column first_delta_at datetime(6) null",
		"add column settled_at datetime(6) null",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("latency timeline migration missing %q", required)
		}
	}

	hcl := strings.ReplaceAll(strings.ToLower(readOfficialModelSchemaFile(t, filepath.Join(root, "database", "schema", "admin.hcl"))), "\r\n", "\n")
	commands := tableBlock(t, hcl, "ai_reply_commands")
	for _, required := range []string{
		`column "request_received_at"`,
		`column "accepted_at"`,
		`column "claimed_at"`,
		`column "claim_source"`,
		`check "chk_ai_reply_claim_source"`,
	} {
		if !strings.Contains(commands, required) {
			t.Errorf("ai_reply_commands missing %q", required)
		}
	}
	attempts := tableBlock(t, hcl, "ai_provider_attempts")
	for _, required := range []string{`column "prepare_started_at"`, `column "first_delta_at"`} {
		if !strings.Contains(attempts, required) {
			t.Errorf("ai_provider_attempts missing %q", required)
		}
	}
	runs := tableBlock(t, hcl, "ai_runs")
	if !strings.Contains(runs, `column "settled_at"`) {
		t.Error("ai_runs missing settled_at")
	}

	reconciliation := strings.ToLower(readOfficialModelSchemaFile(t, filepath.Join(root, "database", "reconciliation", "030_verify_schema.sql")))
	for _, column := range []string{
		"'ai_reply_commands','request_received_at'",
		"'ai_reply_commands','accepted_at'",
		"'ai_reply_commands','claimed_at'",
		"'ai_reply_commands','claim_source'",
		"'ai_provider_attempts','prepare_started_at'",
		"'ai_provider_attempts','first_delta_at'",
		"'ai_runs','settled_at'",
	} {
		if !strings.Contains(reconciliation, column) {
			t.Errorf("schema reconciliation missing %q", column)
		}
	}
}
