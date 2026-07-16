package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAIBackfillPreservesExplicitSourceEvidence(t *testing.T) {
	root := backendRoot(t)
	backfill, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "021_backfill_ai.sql"))
	if err != nil {
		t.Fatal(err)
	}
	verify, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "033_verify_ai.sql"))
	if err != nil {
		t.Fatal(err)
	}
	cosVerifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-cos-references.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.ToLower(string(backfill) + "\n" + string(verify))
	for _, required := range []string{
		"legacy:ai-run:", "ai_image_task_", "ai_text_task_", "canvas_video_task_", "canvas_audio_",
		"retired_image_evidence_complete", "ai_run_source_identity", "ai_run_idempotency_unique",
		"ai_image_source_target_counts", "ai_image_source_target_hash",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("AI reconciliation missing evidence rule %q", required)
		}
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
		regexp.MustCompile(`(?i)\bdrop\s+table\b`),
		regexp.MustCompile(`(?i)\btenant\b`),
	} {
		if forbidden.Match(backfill) {
			t.Errorf("AI backfill contains forbidden SQL %q", forbidden)
		}
	}
	verifierBody := strings.ToLower(string(cosVerifier))
	for _, required := range []string{"cos-references", "$database", "$outputpath", "app_secret", "mysql_dsn"} {
		if !strings.Contains(verifierBody, required) {
			t.Errorf("COS verifier wrapper missing %q", required)
		}
	}
}
