package replycommand

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
)

const durableWorkIntegrationEnv = "ADMIN_DURABLE_WORK_INTEGRATION"

func TestIdempotencyKeyIsStableAndConversationScoped(t *testing.T) {
	first := idempotencyKey(31, "request-7")
	if len(first) != 64 || first != idempotencyKey(31, "request-7") {
		t.Fatalf("unstable key %q", first)
	}
	if first == idempotencyKey(32, "request-7") || first == idempotencyKey(31, "request-8") {
		t.Fatal("idempotency key did not scope conversation and request")
	}
}

func TestCreateReplyRollsBackFailuresAndReturnsOriginalDuplicate(t *testing.T) {
	db := openReplyIntegrationDB(t)
	fixture := createReplyFixture(t, db)
	repository := NewGormRepository(db)
	ctx := context.Background()

	invalidJSON := "{"
	_, err := repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      "message-insert-failure",
		Content:        "message must roll back",
		MetaJSON:       &invalidJSON,
	})
	if err == nil {
		t.Fatal("expected invalid message JSON to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, "message-insert-failure", "message must roll back", 0, 0)

	maxRequestID := strings.Repeat("界", 128)
	if _, err := repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      maxRequestID,
		Content:        "128 character request ID",
	}); err != nil {
		t.Fatalf("128-character request_id was rejected: %v", err)
	}
	assertReplyRows(t, db, fixture.conversationID, maxRequestID, "128 character request ID", 1, 1)

	tooLongRequestID := strings.Repeat("界", 129)
	_, err = repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      tooLongRequestID,
		Content:        "command failure must roll back message",
	})
	if err == nil {
		t.Fatal("expected oversized command request_id to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, tooLongRequestID, "command failure must roll back message", 0, 0)

	collisionKey := fmt.Sprintf("p05-collision-%d", time.Now().UnixNano())
	insertCommandKeyBlocker(t, db, fixture, collisionKey)
	repository.idempotencyKey = func(int64, string) string { return collisionKey }
	_, err = repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      "command-insert-failure",
		Content:        "unique command failure must roll back message",
	})
	if err == nil {
		t.Fatal("expected command unique identity conflict to fail")
	}
	assertReplyRows(t, db, fixture.conversationID, "command-insert-failure", "unique command failure must roll back message", 0, 0)
	repository.idempotencyKey = idempotencyKey

	created, err := repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      "request-1",
		Content:        "original content",
	})
	if err != nil {
		t.Fatalf("CreateReply: %v", err)
	}
	if created.CommandID == 0 || created.UserMessageID == 0 || created.RequestID != "request-1" || created.State != StatePending {
		t.Fatalf("unexpected create result: %+v", created)
	}

	duplicate, err := repository.CreateReply(ctx, CreateReplyInput{
		ConversationID: fixture.conversationID,
		UserID:         fixture.userID,
		RequestID:      "request-1",
		Content:        "duplicate content must not be inserted",
	})
	if err != nil {
		t.Fatalf("duplicate CreateReply: %v", err)
	}
	if duplicate.CommandID != created.CommandID || duplicate.UserMessageID != created.UserMessageID || duplicate.State != StatePending {
		t.Fatalf("duplicate did not return original identity: created=%+v duplicate=%+v", created, duplicate)
	}
	assertReplyRows(t, db, fixture.conversationID, "request-1", "original content", 1, 1)
	assertReplyRows(t, db, fixture.conversationID, "request-1", "duplicate content must not be inserted", 1, 0)
}

type replyFixture struct {
	userID         int64
	agentID        int64
	conversationID int64
}

func openReplyIntegrationDB(t *testing.T) *database.Client {
	t.Helper()
	if os.Getenv(durableWorkIntegrationEnv) != "1" {
		t.Skip("Docker-only durable work integration test")
	}
	db, err := database.Open(config.MySQLConfig{
		DSN:             os.Getenv("MYSQL_DSN"),
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open integration MySQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createReplyFixture(t *testing.T, db *database.Client) replyFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprint(time.Now().UnixNano())
	insert := func(query string, args ...any) int64 {
		result, err := db.SQL.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("insert reply fixture: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("reply fixture id: %v", err)
		}
		return id
	}
	userID := insert("INSERT INTO users (username, status, is_del) VALUES (?, 1, 2)", "p05-reply-"+suffix)
	agentID := insert("INSERT INTO ai_agents (provider_id, name, scenes_json, status, is_del) VALUES (0, ?, JSON_ARRAY('chat'), 1, 2)", "p05-reply-"+suffix)
	conversationID := insert("INSERT INTO ai_conversations (user_id, agent_id, title, is_del) VALUES (?, ?, '', 2)", userID, agentID)
	t.Cleanup(func() {
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM realtime_events WHERE target_type = 'user' AND target_id = ?", fmt.Sprint(userID))
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_provider_attempts WHERE command_id IN (SELECT id FROM ai_reply_commands WHERE conversation_id = ?)", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_reply_commands WHERE conversation_id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_messages WHERE conversation_id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_conversations WHERE id = ?", conversationID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM ai_agents WHERE id = ?", agentID)
		_, _ = db.SQL.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	})
	return replyFixture{userID: userID, agentID: agentID, conversationID: conversationID}
}

func assertReplyRows(t *testing.T, db *database.Client, conversationID int64, requestID string, content string, wantCommands int, wantMessages int) {
	t.Helper()
	var commandCount int
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_reply_commands WHERE conversation_id = ? AND request_id = ?", conversationID, requestID).Scan(&commandCount); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if commandCount != wantCommands {
		t.Fatalf("commands for %q = %d, want %d", requestID, commandCount, wantCommands)
	}
	var messageCount int
	if err := db.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_messages WHERE conversation_id = ? AND content = ?", conversationID, content).Scan(&messageCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != wantMessages {
		t.Fatalf("messages for %q = %d, want %d", content, messageCount, wantMessages)
	}
}

func insertCommandKeyBlocker(t *testing.T, db *database.Client, fixture replyFixture, key string) {
	t.Helper()
	_, err := db.SQL.ExecContext(context.Background(), `
INSERT INTO ai_reply_commands
  (request_id, idempotency_key, platform, user_id, conversation_id, user_message_id, state, max_attempts, next_attempt_at)
VALUES
	(?, ?, 'admin', ?, ?, ?, 'failed', 3, CURRENT_TIMESTAMP(6))`, "blocker", key, fixture.userID, fixture.conversationID, -time.Now().UnixNano())
	if err != nil {
		t.Fatalf("reserve command idempotency key: %v", err)
	}
}
