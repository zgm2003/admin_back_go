package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIConsumerInteractionsSchema(t *testing.T) {
	root := backendRoot(t)
	schema := readArchitectureText(t, "database/schema/admin.hcl")
	migrationPath := filepath.Join(root, "database", "migrations", "202607270102_ai_consumer_interactions.sql")
	migrationBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("read consumer interaction migration: %v", err)
	}
	migration := string(migrationBytes)

	t.Run("canonical fields and index", func(t *testing.T) {
		conversation := hclTableBlock(t, schema, "ai_conversations")
		cursor := hclConsumerInteractionBlock(t, conversation, `column "last_read_message_id"`)
		for _, required := range []string{"type     = bigint", "unsigned = true", "null     = false", "default  = 0"} {
			if !strings.Contains(cursor, required) {
				t.Errorf("ai_conversations.last_read_message_id missing %q", required)
			}
		}
		if strings.Contains(conversation, "columns     = [column.last_read_message_id]") {
			t.Fatal("ai_conversations.last_read_message_id must not have a cursor foreign key")
		}

		run := hclTableBlock(t, schema, "ai_runs")
		likedAt := hclConsumerInteractionBlock(t, run, `column "liked_at"`)
		for _, required := range []string{"type = datetime(6)", "null = true"} {
			if !strings.Contains(likedAt, required) {
				t.Errorf("ai_runs.liked_at missing %q", required)
			}
		}

		messages := hclTableBlock(t, schema, "ai_messages")
		indexColumns := "columns = [column.conversation_id, column.is_del, column.role, column.id]"
		if count := strings.Count(messages, indexColumns); count != 1 {
			t.Errorf("ai_messages must have exactly one conversation/is_del/role/id index, found %d", count)
		}
	})

	t.Run("guarded migration", func(t *testing.T) {
		for _, required := range []string{
			"CREATE TEMPORARY TABLE `_ai_consumer_interactions_guard`",
			"CHECK (`violations` = 0)",
			"information_schema`.`COLUMNS",
			"information_schema`.`STATISTICS",
			"ADD COLUMN `last_read_message_id` BIGINT UNSIGNED NOT NULL DEFAULT 0",
			"ADD COLUMN `liked_at` DATETIME(6) NULL",
			"ADD KEY `idx_ai_messages_conversation_del_role_id` (`conversation_id`, `is_del`, `role`, `id`)",
		} {
			if !strings.Contains(migration, required) {
				t.Errorf("consumer interaction migration missing %q", required)
			}
		}
		if strings.Contains(migration, "FOREIGN KEY (`last_read_message_id`)") {
			t.Fatal("consumer interaction migration must not add a cursor foreign key")
		}
	})

	t.Run("additive scope", func(t *testing.T) {
		upper := strings.ToUpper(migration)
		for _, forbidden := range []string{
			"CREATE TABLE ",
			"CREATE DATABASE",
			"CREATE TRIGGER",
			"CREATE USER",
			"GRANT ",
			"REVOKE ",
			"PERMISSIONS",
			"ROLE_PERMISSIONS",
			"BACKUP",
			"UPDATE `AI_",
			"DELETE FROM `AI_",
			"INSERT INTO `AI_",
			"REPLACE INTO `AI_",
			"TRUNCATE TABLE",
			"DROP TABLE",
			"DROP COLUMN",
			"DROP INDEX",
			"DROP KEY",
			"RENAME TABLE",
		} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("consumer interaction migration contains forbidden operation %q", forbidden)
			}
		}
	})
}

func hclConsumerInteractionBlock(t *testing.T, parent, marker string) string {
	t.Helper()
	start := strings.Index(parent, marker)
	if start < 0 {
		t.Errorf("canonical schema missing %s", marker)
		return ""
	}
	depth := 0
	for index := start; index < len(parent); index++ {
		switch parent[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return parent[start : index+1]
			}
		}
	}
	t.Errorf("canonical schema block %s has unbalanced braces", marker)
	return ""
}
