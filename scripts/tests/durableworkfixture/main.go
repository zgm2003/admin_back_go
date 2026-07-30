package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/module/ai/replycommand"
	modulerealtime "admin_back_go/internal/module/realtime"
)

type options struct {
	mode           string
	providerURL    string
	userID         int64
	conversationID int64
	commandID      uint64
	requestID      string
	content        string
	afterSequence  uint64
}

func main() {
	var input options
	flag.StringVar(&input.mode, "mode", "", "setup, create, status, cancel, or resume")
	flag.StringVar(&input.providerURL, "provider-url", "", "test OpenAI-compatible base URL")
	flag.Int64Var(&input.userID, "user-id", 0, "fixture user ID")
	flag.Int64Var(&input.conversationID, "conversation-id", 0, "fixture conversation ID")
	flag.Uint64Var(&input.commandID, "command-id", 0, "reply command ID")
	flag.StringVar(&input.requestID, "request-id", "", "reply request ID")
	flag.StringVar(&input.content, "content", "", "user message content")
	flag.Uint64Var(&input.afterSequence, "after-sequence", 0, "durable realtime cursor")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := database.Open(config.MySQLConfig{
		DSN:             strings.TrimSpace(os.Getenv("MYSQL_DSN")),
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	var result any
	switch strings.TrimSpace(input.mode) {
	case "setup":
		result, err = setupFixture(ctx, client, input.providerURL)
	case "create":
		result, err = createReply(ctx, client, input)
	case "status":
		result, err = commandStatus(ctx, client, input.commandID)
	case "cancel":
		result, err = cancelReply(ctx, client, input)
	case "resume":
		result, err = resumeEvents(ctx, client, input.userID, input.afterSequence)
	default:
		err = errors.New("mode is required")
	}
	if err != nil {
		log.Fatal(err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("P05_RESULT %s\n", payload)
}

func setupFixture(ctx context.Context, client *database.Client, providerURL string) (map[string]any, error) {
	providerURL = strings.TrimRight(strings.TrimSpace(providerURL), "/")
	if providerURL == "" {
		return nil, errors.New("provider-url is required")
	}
	keys, err := secretkey.NewKeyRing(os.Getenv("APP_SECRET"))
	if err != nil {
		return nil, err
	}
	apiKey, err := secretbox.New(keys.SecretboxKey()).Encrypt("p05-test-api-key")
	if err != nil {
		return nil, err
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	tx, err := client.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	insertID := func(query string, args ...any) (int64, error) {
		result, execErr := tx.ExecContext(ctx, query, args...)
		if execErr != nil {
			return 0, execErr
		}
		return result.LastInsertId()
	}
	providerID, err := insertID(`INSERT INTO ai_providers
(name,engine_type,base_url,api_key_enc,api_key_hint,health_status,last_check_error,last_model_sync_status,last_model_sync_error,status,is_del)
VALUES (?,?,?,?,?,'ok','','ok','',1,2)`, "p05-provider-"+suffix, "openai", providerURL, apiKey, "***-key")
	if err != nil {
		return nil, err
	}
	agentID, err := insertID(`INSERT INTO ai_agents
(provider_id,name,model_id,model_display_name,scenes_json,system_prompt,avatar,status,is_del)
VALUES (?,?,?, ?,JSON_ARRAY('chat'),'','','1','2')`, providerID, "p05-agent-"+suffix, "p05-model", "P05 Model")
	if err != nil {
		return nil, err
	}
	userID, err := insertID("INSERT INTO users (username,status,is_del) VALUES (?,1,2)", "p05-user-"+suffix)
	if err != nil {
		return nil, err
	}
	conversationID, err := insertID("INSERT INTO ai_conversations (user_id,agent_id,title,is_del) VALUES (?,?,?,2)", userID, agentID, "P05 durable work")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{
		"provider_id": providerID, "agent_id": agentID,
		"user_id": userID, "conversation_id": conversationID,
	}, nil
}

func createReply(ctx context.Context, client *database.Client, input options) (map[string]any, error) {
	if input.userID <= 0 || input.conversationID <= 0 || strings.TrimSpace(input.requestID) == "" {
		return nil, errors.New("user-id, conversation-id, and request-id are required")
	}
	repository := replycommand.NewGormRepository(client)
	created, err := repository.CreateReply(ctx, replycommand.CreateReplyInput{
		ConversationID: input.conversationID,
		UserID:         input.userID,
		RequestID:      input.requestID,
		Content:        input.content,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"command_id": created.CommandID, "user_message_id": created.UserMessageID,
		"request_id": created.RequestID, "state": created.State,
	}, nil
}

func commandStatus(ctx context.Context, client *database.Client, commandID uint64) (map[string]any, error) {
	if commandID == 0 {
		return nil, errors.New("command-id is required")
	}
	var state string
	var requestID string
	var leaseOwner sql.NullString
	var leaseExpiresAt sql.NullTime
	var assistantMessageID sql.NullInt64
	var leaseToken uint64
	err := client.SQL.QueryRowContext(ctx, `SELECT state,request_id,lease_token,lease_owner,lease_expires_at,assistant_message_id
FROM ai_reply_commands WHERE id=?`, commandID).Scan(&state, &requestID, &leaseToken, &leaseOwner, &leaseExpiresAt, &assistantMessageID)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for name, query := range map[string]string{
		"assistant_count": "SELECT COUNT(*) FROM ai_messages WHERE reply_command_id=? AND role=2",
		"attempt_count":   "SELECT COUNT(*) FROM ai_provider_attempts WHERE command_id=?",
		"event_count":     "SELECT COUNT(*) FROM realtime_events WHERE request_id=?",
	} {
		argument := any(commandID)
		if name == "event_count" {
			argument = requestID
		}
		var count int64
		if err := client.SQL.QueryRowContext(ctx, query, argument).Scan(&count); err != nil {
			return nil, err
		}
		counts[name] = count
	}
	result := map[string]any{
		"state": state, "request_id": requestID, "lease_token": leaseToken,
		"lease_owner": leaseOwner.String, "assistant_message_id": assistantMessageID.Int64,
		"assistant_count": counts["assistant_count"], "attempt_count": counts["attempt_count"],
		"event_count": counts["event_count"],
	}
	if leaseExpiresAt.Valid {
		result["lease_expires_at"] = leaseExpiresAt.Time.UTC().Format(time.RFC3339Nano)
	} else {
		result["lease_expires_at"] = ""
	}
	return result, nil
}

func cancelReply(ctx context.Context, client *database.Client, input options) (map[string]any, error) {
	if input.commandID == 0 || input.userID <= 0 || input.conversationID <= 0 || strings.TrimSpace(input.requestID) == "" {
		return nil, errors.New("command-id, user-id, conversation-id, and request-id are required")
	}
	events := modulerealtime.NewGormRepository(client, modulerealtime.DefaultRegistry())
	sink := modulerealtime.NewDurableEventSink(events, infrarealtime.NoopPublisher{}, slog.Default())
	repository := replycommand.NewGormRepository(client, replycommand.WithDurableEventSink(sink))
	result, err := repository.RequestCancel(ctx, replycommand.RequestCancelInput{
		ConversationID: input.conversationID,
		UserID:         input.userID,
		RequestID:      input.requestID,
		DeliveredSeq:   0,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	redisDB, err := strconv.Atoi(strings.TrimSpace(os.Getenv("REDIS_DB")))
	if err != nil {
		return nil, err
	}
	redis, err := redisclient.Open(config.RedisConfig{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	if err != nil {
		return nil, err
	}
	defer func() { _ = redis.Close() }()
	if err := replycommand.NewRedisCancelPublisher(redis).PublishCancel(ctx, result.CommandID); err != nil {
		return nil, err
	}
	return map[string]any{
		"command_id": result.CommandID, "status": result.Status,
		"assistant_message_id": result.AssistantMessageID,
		"settlement_pending":   result.SettlementPending,
		"delivery_consistent":  result.DeliveryConsistent,
		"stop_delivery_seq":    result.StopDeliverySeq,
	}, nil
}

func resumeEvents(ctx context.Context, client *database.Client, userID int64, after uint64) (map[string]any, error) {
	if userID <= 0 {
		return nil, errors.New("user-id is required")
	}
	repository := modulerealtime.NewGormRepository(client, modulerealtime.DefaultRegistry())
	result, err := repository.ResumeUser(ctx, modulerealtime.ResumeQuery{UserID: userID, AfterSequence: after, Limit: modulerealtime.MaxResumeLimit})
	if err != nil {
		return nil, err
	}
	sequences := make([]uint64, 0, len(result.Events))
	types := make([]string, 0, len(result.Events))
	for _, event := range result.Events {
		sequences = append(sequences, event.Sequence)
		types = append(types, event.EventType)
	}
	return map[string]any{
		"count": len(result.Events), "sequences": sequences, "types": types,
		"latest_sequence": result.LatestSequence, "resync_required": result.ResyncRequired,
	}, nil
}
