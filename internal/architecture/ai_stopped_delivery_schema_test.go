package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIStoppedDeliverySchemaContract(t *testing.T) {
	root := backendRoot(t)
	schema := readArchitectureText(t, "database/schema/admin.hcl")
	migrationPath := filepath.Join(root, "database", "migrations", "202607300101_ai_chat_stopped_delivery.sql")
	migrationBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read stopped delivery migration: %v", err)
	}
	migration := string(migrationBytes)
	schemaVerification := readArchitectureText(t, "database/reconciliation/030_verify_schema.sql")
	relations := readArchitectureText(t, "database/reconciliation/031_verify_relations.sql")

	commands := hclTableBlock(t, schema, "ai_reply_commands")
	for _, required := range []string{
		`column "delivery_seq" {`,
		"type     = int",
		"unsigned = true",
		"default  = 0",
		`column "stop_delivery_seq" {`,
		`check "chk_ai_reply_delivery_seq" {`,
	} {
		if !strings.Contains(commands, required) {
			t.Errorf("ai_reply_commands missing %q", required)
		}
	}

	messages := hclTableBlock(t, schema, "ai_messages")
	for _, required := range []string{
		`column "delivery_state" {`,
		"type = varchar(16)",
		"null = true",
		`check "chk_ai_messages_delivery_state" {`,
	} {
		if !strings.Contains(messages, required) {
			t.Errorf("ai_messages missing %q", required)
		}
	}

	deliveryChunks := hclTableBlock(t, schema, "ai_reply_delivery_chunks")
	if strings.Count(deliveryChunks, "primary_key {") != 1 ||
		!strings.Contains(deliveryChunks, "columns = [column.command_id, column.delivery_seq]") {
		t.Error("ai_reply_delivery_chunks must have the command_id/delivery_seq composite primary key")
	}
	if strings.Contains(deliveryChunks, "index ") {
		t.Error("ai_reply_delivery_chunks must not have secondary indexes")
	}
	if !strings.Contains(deliveryChunks, `foreign_key "fk_ai_reply_delivery_chunks_command"`) {
		t.Error("ai_reply_delivery_chunks missing command foreign key")
	}
	for _, required := range []string{
		`check "chk_ai_reply_delivery_chunk_seq"`,
		`check "chk_ai_reply_delivery_chunk_size"`,
	} {
		if !strings.Contains(deliveryChunks, required) {
			t.Errorf("ai_reply_delivery_chunks missing %q", required)
		}
	}

	for _, required := range []string{
		"CREATE TEMPORARY TABLE `_ai_stopped_delivery_guard`",
		"CHECK (`violations` = 0)",
		"`state` IN ('pending', 'claimed', 'running')",
		"`state` IN ('prepared', 'dispatched')",
		"DELETE FROM `realtime_events` WHERE `event_type` = 'ai.response.canceled.v1'",
		"UPDATE `ai_reply_commands` SET `stop_delivery_seq` = 0 WHERE `cancel_requested_at` IS NOT NULL",
		"UPDATE `ai_messages` SET `delivery_state` = 'completed' WHERE `role` = 2",
		"CREATE TABLE `ai_reply_delivery_chunks`",
		"PRIMARY KEY (`command_id`, `delivery_seq`)",
		"CONSTRAINT `fk_ai_reply_delivery_chunks_command`",
		"DROP TEMPORARY TABLE `_ai_stopped_delivery_guard`",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	if firstDDL := strings.Index(migration, "ALTER TABLE"); firstDDL >= 0 &&
		strings.Index(migration, "CREATE TEMPORARY TABLE `_ai_stopped_delivery_guard`") > firstDDL {
		t.Error("stopped delivery guard must precede permanent DDL")
	}
	if !strings.Contains(schemaVerification, "ai_reply_delivery_chunks") {
		t.Error("schema verification missing ai_reply_delivery_chunks")
	}
	indexInvariantStart := strings.Index(schemaVerification, "SELECT 'ai_reply_delivery_chunk_indexes'")
	indexInvariantEnd := -1
	if indexInvariantStart >= 0 {
		if relativeEnd := strings.Index(schemaVerification[indexInvariantStart:], "\n\nSELECT "); relativeEnd >= 0 {
			indexInvariantEnd = indexInvariantStart + relativeEnd
		}
	}
	if indexInvariantStart < 0 || indexInvariantEnd < 0 ||
		!strings.Contains(schemaVerification[indexInvariantStart:indexInvariantEnd], "COUNT(DISTINCT index_name)") {
		t.Error("delivery chunk index invariant must count indexes, not composite-key columns")
	}
	if !strings.Contains(relations, "fk_ai_reply_delivery_chunks_command") {
		t.Error("relation verification missing fk_ai_reply_delivery_chunks_command")
	}
}
