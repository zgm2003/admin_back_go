package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReconciliationExpandIsNonDestructiveAndComplete(t *testing.T) {
	root := backendRoot(t)
	attributes, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(attributes)), "/database/reconciliation/*.sql text eol=lf") {
		t.Error("reconciliation SQL must use stable LF bytes for ledger checksums")
	}
	expandPath := filepath.Join(root, "database", "reconciliation", "010_expand_core.sql")
	verifyPath := filepath.Join(root, "database", "reconciliation", "030_verify_schema.sql")
	expand, err := os.ReadFile(expandPath)
	if err != nil {
		t.Fatal(err)
	}
	verify, err := os.ReadFile(verifyPath)
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.ToLower(string(expand) + "\n" + string(verify))
	for _, required := range []string{
		"export_tasks", "platform", "kind", "object_key",
		"ai_runs", "input_snapshot", "idempotency_key",
		"ai_image_tasks", "ai_image_files", "ai_text_tasks", "ai_video_tasks", "ai_assets",
		"payment_callback_events", "total_consume_cents", "verify_code_ttl_minutes",
		"authz_principal_versions", "ai_reply_commands", "ai_provider_attempts", "realtime_events",
		"source_task_id", "claim_owner", "claim_token", "claim_expires_at",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("missing approved identifier %q", required)
		}
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bdrop\s+table\b`),
		regexp.MustCompile(`(?i)\bdrop\s+column\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
		regexp.MustCompile(`(?i)\btenant\b`),
	} {
		if forbidden.Match(expand) {
			t.Errorf("expand contains forbidden SQL %q", forbidden)
		}
	}
	for _, requiredEvidence := range []string{
		"column_type", "is_nullable", "column_default",
		"non_unique", "seq_in_index", "group_concat", "check_clause",
	} {
		if !strings.Contains(strings.ToLower(string(verify)), requiredEvidence) {
			t.Errorf("schema verifier does not inspect %q", requiredEvidence)
		}
	}
}
