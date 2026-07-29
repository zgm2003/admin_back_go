package architecture

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestUnifiedAIRunMigrationShape(t *testing.T) {
	body, err := os.ReadFile("../../database/legacy-migrations/20260607_unified_ai_run_records.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	required := []string{
		"ADD COLUMN `platform` VARCHAR(32) NULL",
		"ADD COLUMN `input_snapshot` MEDIUMTEXT NULL",
		"MODIFY COLUMN `platform` VARCHAR(32) NOT NULL",
		"MODIFY COLUMN `input_snapshot` MEDIUMTEXT NOT NULL",
		"MODIFY COLUMN `conversation_id` INT UNSIGNED NULL",
		"MODIFY COLUMN `user_message_id` BIGINT UNSIGNED NULL",
		"MODIFY COLUMN `assistant_message_id` BIGINT UNSIGNED NULL",
		"CREATE TABLE `ai_text_tasks`",
		"ALTER TABLE `ai_image_tasks` ADD COLUMN `platform` VARCHAR(32) NULL",
	}
	for _, want := range required {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	for _, bad := range []string{"ai_billing_records", "ai_billing_rules", "usage_json", "`cost` DECIMAL", "DEFAULT 'admin'", "DEFAULT 'chat'", "DEFAULT 'ai_chat_message'"} {
		if strings.Contains(sql, bad) {
			t.Fatalf("migration must not contain %q", bad)
		}
	}
}

func TestAIRunSourceFieldCleanupMigrationShape(t *testing.T) {
	body, err := os.ReadFile("../../database/legacy-migrations/20260608_ai_run_source_field_cleanup.sql")
	if err != nil {
		t.Fatalf("read cleanup migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"ADD COLUMN `run_id` BIGINT UNSIGNED NULL",
		"UPDATE `canvas_video_tasks` t",
		"DROP INDEX `uk_ai_runs_source_request`",
		"DROP INDEX `idx_ai_runs_platform_modality_created`",
		"DROP INDEX `idx_ai_runs_source`",
		"DROP COLUMN `modality`",
		"DROP COLUMN `source_type`",
		"DROP COLUMN `source_id`",
		"DROP COLUMN `usage_status`",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cleanup migration missing %q", want)
		}
	}
}

func TestAIChatBootstrapWiresUnifiedRunRecorderAndDurableTextGeneration(t *testing.T) {
	for _, path := range []string{"../../internal/platform/admin/build.go", "../../internal/runtime/worker.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(body)
		for _, want := range []string{
			"aiRunRecorder := airun.NewRecorder(aiRunRepository, nil)",
			"aiTextTasks := aitext.NewGormStore(resources.DB)",
			"aiTextService := aitext.NewService",
		} {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
		if !strings.Contains(source, "RunRecorder:") || !strings.Contains(source, "aiRunRecorder") {
			t.Fatalf("%s missing AI run recorder wiring", path)
		}
		if !strings.Contains(source, "TextGeneration:") || !strings.Contains(source, "aiTextService") {
			t.Fatalf("%s missing durable AI text generation wiring", path)
		}
		if strings.Contains(source, "TextTasks:") {
			t.Fatalf("%s still wires the obsolete legacy text task writer", path)
		}
	}
}

func TestAITextStoreDoesNotExposeLegacyDirectWriters(t *testing.T) {
	body, err := os.ReadFile("../../internal/module/ai/text/store.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, forbidden := range []string{
		"type Store interface",
		"func (s *GormStore) Create(",
		"func (s *GormStore) Complete(",
		"func (s *GormStore) Fail(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("AI text store still exposes legacy direct writer %q", forbidden)
		}
	}
}

func TestWorkerWiresPaidImageExecutorAndRecoveryReconciler(t *testing.T) {
	body, err := os.ReadFile("../../internal/runtime/worker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"paidImageExecutor := newPaidImageTaskExecutor",
		"imageReconciler",
		"aiimage.NewReconciler",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("worker missing paid image runtime wiring %q", required)
		}
	}
	if !regexp.MustCompile(`Executor:\s+paidImageExecutor`).MatchString(source) {
		t.Fatal("worker does not wire the paid image executor")
	}
}
