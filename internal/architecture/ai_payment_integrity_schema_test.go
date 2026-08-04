package architecture

import (
	"strings"
	"testing"
)

const (
	aiPaymentIntegrityExpand   = "database/migrations/202608040101_ai_payment_integrity_expand.sql"
	aiPaymentIntegrityBackfill = "database/migrations/202608040102_ai_payment_integrity_backfill.sql"
	aiPaymentIntegrityContract = "database/migrations/202608040103_ai_payment_integrity_contract.sql"
)

func TestAIPaymentIntegrityExpandAddsOnlyNullableIdentityColumns(t *testing.T) {
	migration := strings.Join(strings.Fields(strings.ToLower(mustReadRepoFile(t, aiPaymentIntegrityExpand))), " ")
	for _, required := range []string{
		"alter table `payment_callback_events` add column `dedupe_key` binary(32) null",
		"alter table `payment_orders` add column `alipay_trade_no_identity` varchar(64) null",
		"alter table `ai_agents` add column `provider_model_id` bigint unsigned null",
		"alter table `ai_reply_commands` add column `run_id` bigint unsigned null",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("Expand missing %q", required)
		}
	}
	for _, forbidden := range []string{" drop column ", " not null", " foreign key ", " unique key ", "update `"} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("Expand contains Contract/backfill operation %q", forbidden)
		}
	}
}

func TestAIPaymentIntegrityBackfillIsFailClosedAndMatchesRuntimeIdentities(t *testing.T) {
	migration := strings.ToLower(mustReadRepoFile(t, aiPaymentIntegrityBackfill))
	for _, required := range []string{
		"check (`violations` = 0)",
		"json_unquote(json_extract(`raw_payload_json`, '$.total_amount'))",
		"unhex(sha2(concat(",
		"char(0)",
		"'payment_callback_v1'",
		"'notify_id'",
		"'callback_facts'",
		"set `alipay_trade_no_identity` = nullif(trim(`alipay_trade_no`), '')",
		"join `ai_provider_models` as provider_model",
		"provider_model.`model_kind` = 'chat'",
		"set agent.`provider_model_id` = provider_model.`id`",
		"join `ai_runs` as run_row on run_row.`user_message_id` = command_row.`user_message_id`",
		"set command_row.`run_id` = run_row.`id`",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("Backfill missing %q", required)
		}
	}
	for _, forbidden := range []string{"coalesce(agent.`provider_model_id`, 0)", "coalesce(command_row.`run_id`, 0)", "delete from `"} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("Backfill hides invalid data with %q", forbidden)
		}
	}
}

func TestAIPaymentIntegrityBackfillRejectsWhitespaceOnlyTradeNumberNormalization(t *testing.T) {
	migration := strings.ToLower(mustReadRepoFile(t, aiPaymentIntegrityBackfill))
	rejectWhitespace := "where binary `alipay_trade_no` <> binary trim(`alipay_trade_no`)"
	rejectAt := strings.Index(migration, rejectWhitespace)
	backfillAt := strings.Index(migration, "set `alipay_trade_no_identity` = nullif(trim(`alipay_trade_no`), '')")
	if rejectAt < 0 || backfillAt < 0 || rejectAt > backfillAt {
		t.Fatal("Backfill must reject alipay_trade_no values with leading or trailing whitespace before deriving the identity")
	}
}

func TestAIPaymentIntegrityGuardsCompareTradeIdentityWithoutNormalization(t *testing.T) {
	for _, migrationPath := range []string{aiPaymentIntegrityBackfill, aiPaymentIntegrityContract} {
		migration := strings.Join(strings.Fields(strings.ToLower(mustReadRepoFile(t, migrationPath))), " ")
		for _, required := range []string{
			"`alipay_trade_no` = '' and `alipay_trade_no_identity` is not null",
			"`alipay_trade_no` <> ''",
			"`alipay_trade_no_identity` is null",
			"binary `alipay_trade_no_identity` <> binary `alipay_trade_no`",
		} {
			if !strings.Contains(migration, required) {
				t.Errorf("%s guard missing %q", migrationPath, required)
			}
		}
		if strings.Contains(migration, "binary `alipay_trade_no_identity` <> binary trim(`alipay_trade_no`)") {
			t.Errorf("%s guard normalizes the stored trade number before comparison", migrationPath)
		}
	}
}

func TestAIPaymentIntegrityContractGuardsThenAddsCoreConstraints(t *testing.T) {
	migration := strings.ToLower(mustReadRepoFile(t, aiPaymentIntegrityContract))
	guard := strings.Index(migration, "check (`violations` = 0)")
	firstAlter := strings.Index(migration, "alter table")
	if guard < 0 || firstAlter < 0 || guard > firstAlter {
		t.Fatal("Contract must install fail-closed guards before DDL")
	}
	for _, required := range []string{
		"unique key `uk_payment_callback_events_dedupe` (`dedupe_key`)",
		"unique key `uk_payment_orders_alipay_trade_identity` (`alipay_trade_no_identity`)",
		"constraint `fk_ai_provider_models_provider`",
		"constraint `fk_ai_agents_provider_model`",
		"constraint `fk_ai_conversations_user`",
		"constraint `fk_ai_conversations_agent`",
		"constraint `fk_ai_reply_commands_run_owner`",
		"constraint `fk_ai_reply_commands_conversation_owner`",
		"constraint `fk_ai_reply_commands_user_message_owner`",
		"constraint `fk_ai_reply_commands_assistant_message_owner`",
		"constraint `fk_ai_provider_attempts_command_run`",
		"constraint `fk_ai_provider_attempts_context_plan_run`",
		"constraint `fk_ai_context_documents_source_message_owner`",
		"constraint `fk_ai_conversation_memories_from_message_owner`",
		"constraint `fk_ai_conversation_memories_through_message_owner`",
		"constraint `fk_ai_conversation_memories_previous_owner`",
		"constraint `fk_ai_image_files_task`",
		"constraint `fk_ai_image_files_related`",
		"unique key `uk_ai_text_tasks_run` (`run_id`)",
		"unique key `uk_ai_image_tasks_run` (`run_id`)",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("Contract missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"drop column `provider_id`", "drop column `model_id`", "drop column `model_display_name`",
		"drop column `notify_id`", "drop column `alipay_trade_no`", "drop table",
	} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("Contract breaks existing data contract with %q", forbidden)
		}
	}
}

func TestAIPaymentIntegrityCanonicalSchemaCarriesIdentityAndOwnership(t *testing.T) {
	schema := mustReadRepoFile(t, "database/schema/admin.hcl")
	markers := map[string][]string{
		"payment_callback_events":  {`column "dedupe_key"`, `index "uk_payment_callback_events_dedupe"`},
		"payment_orders":           {`column "alipay_trade_no_identity"`, `index "uk_payment_orders_alipay_trade_identity"`},
		"ai_provider_models":       {`foreign_key "fk_ai_provider_models_provider"`, `index "uk_ai_provider_models_id_provider_model"`},
		"ai_agents":                {`column "provider_model_id"`, `foreign_key "fk_ai_agents_provider_model"`},
		"ai_conversations":         {`foreign_key "fk_ai_conversations_user"`, `foreign_key "fk_ai_conversations_agent"`, `index "uk_ai_conversations_id_user"`},
		"ai_messages":              {`index "uk_ai_messages_id_conversation"`},
		"ai_reply_commands":        {`column "run_id"`, `foreign_key "fk_ai_reply_commands_run_owner"`, `foreign_key "fk_ai_reply_commands_conversation_owner"`, `foreign_key "fk_ai_reply_commands_user_message_owner"`, `foreign_key "fk_ai_reply_commands_assistant_message_owner"`, `index "uk_ai_reply_commands_id_run"`},
		"ai_runs":                  {`index "uk_ai_runs_command_owner"`},
		"ai_provider_attempts":     {`foreign_key "fk_ai_provider_attempts_command_run"`, `foreign_key "fk_ai_provider_attempts_context_plan_run"`},
		"ai_context_plans":         {`index "uk_ai_context_plans_id_run"`},
		"ai_context_documents":     {`foreign_key "fk_ai_context_documents_source_message_owner"`},
		"ai_conversation_memories": {`foreign_key "fk_ai_conversation_memories_from_message_owner"`, `foreign_key "fk_ai_conversation_memories_through_message_owner"`, `foreign_key "fk_ai_conversation_memories_previous_owner"`, `index "uk_ai_conversation_memories_owner"`},
		"ai_image_files":           {`foreign_key "fk_ai_image_files_task"`, `foreign_key "fk_ai_image_files_related"`},
		"ai_text_tasks":            {`index "uk_ai_text_tasks_run"`},
		"ai_image_tasks":           {`index "uk_ai_image_tasks_run"`},
	}
	for tableName, required := range markers {
		block := hclTableBlock(t, schema, tableName)
		for _, marker := range required {
			if !strings.Contains(block, marker) {
				t.Errorf("%s missing %s", tableName, marker)
			}
		}
	}
}
