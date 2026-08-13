package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDatabaseBaselineSchemaContract(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "schema.sql"))
	if err != nil {
		t.Fatalf("read database/schema.sql: %v", err)
	}
	normalized := strings.ToLower(string(schema))

	for _, required := range []string{
		"create table `users`",
		"create table `ai_context_plans`",
		"create table `ai_reply_delivery_chunks`",
		"create table `ai_run_dashboard_facts`",
		"create table `payment_orders`",
		"create table `schema_migrations`",
		"constraint `fk_mail_log_verification_codes_mail_log`",
		"constraint `fk_ai_context_documents_source_message_owner`",
		"constraint `fk_ai_reply_delivery_chunks_command`",
		"constraint `fk_ai_provider_attempts_context_plan_run`",
		"constraint `chk_ai_context_plans_terminal_shape`",
		"constraint `chk_ai_runs_status`",
		"unique key `uk_ai_conversation_memories_owner`",
		"unique key `uk_payment_callback_events_dedupe`",
		"unique key `uk_payment_orders_alipay_trade_identity`",
		"unique key `uk_wallet_transaction_source`",
		"key `idx_ai_runs_model_created`",
		"key `idx_ai_runs_billing_created`",
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

func TestDatabaseBaselineSeedContract(t *testing.T) {
	seed, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "seed.sql"))
	if err != nil {
		t.Fatalf("read database/seed.sql: %v", err)
	}
	normalized := strings.ToLower(string(seed))
	allowedTables := map[string]bool{
		"permissions":       true,
		"roles":             true,
		"role_permissions":  true,
		"auth_platforms":    true,
		"system_settings":   true,
		"cron_task":         true,
		"ai_tools":          true,
		"schema_migrations": true,
	}
	inserts := regexp.MustCompile(`(?i)insert\s+into\s+`+"`"+`([a-z0-9_]+)`+"`").FindAllStringSubmatch(string(seed), -1)
	seen := make(map[string]bool, len(inserts))
	for _, insert := range inserts {
		table := strings.ToLower(insert[1])
		if !allowedTables[table] {
			t.Errorf("seed inserts forbidden table %q", table)
		}
		seen[table] = true
	}
	for table := range allowedTables {
		if !seen[table] {
			t.Errorf("seed does not initialize %q", table)
		}
	}

	for _, required := range []string{
		"start transaction",
		"commit",
		"202608130001",
		"auth.captcha.ttl_minutes",
		"auth.captcha.slide_padding",
		"upload.token.ttl_minutes",
		"admin_user_count",
		"ai_run_timeout",
		"notification_task_scheduler",
		"export_cleanup_expired",
		"realtime_event_retention_cleanup",
		"payment_sync_pending_order",
		"payment_close_expired_order",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("seed missing required fact %q", required)
		}
	}
	for _, forbidden := range []string{
		"insert into `users`",
		"insert into `ai_providers`",
		"insert into `ai_provider_models`",
		"insert into `ai_agents`",
		"insert into `ai_context_",
		"insert into `ai_conversations`",
		"insert into `ai_messages`",
		"insert into `ai_runs`",
		"insert into `payment_",
		"insert into `upload_driver`",
		"insert into `upload_rule`",
		"insert into `upload_setting`",
		"insert into `operation_logs`",
		"insert into `cron_task_log`",
		"api_key_enc",
		"secret_id_enc",
		"secret_key_enc",
		"app_private_key_enc",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("seed contains forbidden fact %q", forbidden)
		}
	}
}
