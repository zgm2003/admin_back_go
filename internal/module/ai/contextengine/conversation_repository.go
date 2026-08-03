package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrConversationTurnRepositoryNotConfigured = errors.New("conversation turn repository not configured")

type GormConversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(client *database.Client) *GormConversationRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormConversationRepository{db: client.Gorm}
}

func NewConversationRepositoryWithDB(db *gorm.DB) *GormConversationRepository {
	if db == nil {
		return nil
	}
	return &GormConversationRepository{db: db}
}

type conversationTurnRow struct {
	ConversationID     uint64  `gorm:"column:conversation_id"`
	ConversationUserID uint64  `gorm:"column:conversation_user_id"`
	ConversationAgent  uint64  `gorm:"column:conversation_agent_id"`
	UserMessageID      uint64  `gorm:"column:user_message_id"`
	UserRole           int     `gorm:"column:user_role"`
	UserContent        string  `gorm:"column:user_content"`
	UserMetaJSON       *string `gorm:"column:user_meta_json"`
	CommandID          uint64  `gorm:"column:command_id"`
	CommandState       string  `gorm:"column:command_state"`
	RunID              uint64  `gorm:"column:run_id"`
	RunStatus          string  `gorm:"column:run_status"`
	RunUserID          uint64  `gorm:"column:run_user_id"`
	RunAgentID         uint64  `gorm:"column:run_agent_id"`
	AssistantMessageID uint64  `gorm:"column:assistant_message_id"`
	AssistantRole      int     `gorm:"column:assistant_role"`
	AssistantContent   string  `gorm:"column:assistant_content"`
	AssistantDelivery  *string `gorm:"column:assistant_delivery"`
}

type conversationToolRow struct {
	ID            uint64  `gorm:"column:id"`
	RunID         uint64  `gorm:"column:run_id"`
	CallID        *string `gorm:"column:call_id"`
	ToolCode      string  `gorm:"column:tool_code"`
	Status        string  `gorm:"column:status"`
	ArgumentsJSON string  `gorm:"column:arguments_json"`
	ResultJSON    *string `gorm:"column:result_json"`
}

func (repository *GormConversationRepository) NewestComplete(
	ctx context.Context,
	conversationID uint64,
	userID uint64,
	exclusiveAnchor *uint64,
) (*ConversationTurn, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrConversationTurnRepositoryNotConfigured
	}
	if conversationID == 0 || userID == 0 || exclusiveAnchor != nil && *exclusiveAnchor == 0 {
		return nil, errTurnInvalid
	}
	query := repository.completeRowsQuery(ctx).
		Where("m.conversation_id = ? AND conversation.user_id = ?", conversationID, userID)
	if exclusiveAnchor != nil {
		query = query.Where("m.id < ?", *exclusiveAnchor)
	}
	var rows []conversationTurnRow
	if err := query.Order("m.id DESC").Limit(1).Scan(&rows).Error; err != nil {
		return nil, err
	}
	turns, err := repository.buildTurns(ctx, rows, conversationID, userID)
	if err != nil || len(turns) == 0 {
		return nil, err
	}
	return &turns[0], nil
}

func (repository *GormConversationRepository) CompleteByAnchors(
	ctx context.Context,
	conversationID uint64,
	userID uint64,
	anchors []uint64,
) ([]ConversationTurn, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrConversationTurnRepositoryNotConfigured
	}
	if conversationID == 0 || userID == 0 {
		return nil, errTurnInvalid
	}
	seen := make(map[uint64]struct{}, len(anchors))
	for _, anchor := range anchors {
		if anchor == 0 {
			return nil, errTurnInvalid
		}
		if _, duplicate := seen[anchor]; duplicate {
			return nil, fmt.Errorf("%w: duplicate anchor", errTurnInvalid)
		}
		seen[anchor] = struct{}{}
	}
	var rows []conversationTurnRow
	query := repository.completeRowsQuery(ctx)
	if len(anchors) == 0 {
		query = query.Where("1 = 0")
	} else {
		query = query.Where("m.id IN ?", anchors)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ConversationID != conversationID || row.ConversationUserID != userID {
			return nil, fmt.Errorf("%w: cross-conversation anchor %d", errTurnInvalid, row.UserMessageID)
		}
	}
	turns, err := repository.buildTurns(ctx, rows, conversationID, userID)
	if err != nil {
		return nil, err
	}
	byAnchor := make(map[uint64]ConversationTurn, len(turns))
	for _, turn := range turns {
		byAnchor[turn.UserMessage.ID] = turn
	}
	ordered := make([]ConversationTurn, 0, len(turns))
	for _, anchor := range anchors {
		if turn, ok := byAnchor[anchor]; ok {
			ordered = append(ordered, turn)
		}
	}
	return ordered, nil
}

func (repository *GormConversationRepository) completeRowsQuery(ctx context.Context) *gorm.DB {
	return repository.db.WithContext(ctx).Table("ai_messages m").
		Select(`conversation.id AS conversation_id,
			conversation.user_id AS conversation_user_id,
			conversation.agent_id AS conversation_agent_id,
			m.id AS user_message_id, m.role AS user_role, m.content AS user_content, m.meta_json AS user_meta_json,
			command_row.id AS command_id, command_row.state AS command_state,
			run_row.id AS run_id, run_row.status AS run_status, run_row.user_id AS run_user_id, run_row.agent_id AS run_agent_id,
			assistant.id AS assistant_message_id, assistant.role AS assistant_role,
			assistant.content AS assistant_content, assistant.delivery_state AS assistant_delivery`).
		Joins("JOIN ai_conversations conversation ON conversation.id = m.conversation_id AND conversation.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_reply_commands command_row ON command_row.user_message_id = m.id AND command_row.conversation_id = m.conversation_id AND command_row.user_id = conversation.user_id AND command_row.assistant_message_id IS NOT NULL").
		Joins("JOIN ai_messages assistant ON assistant.id = command_row.assistant_message_id AND assistant.conversation_id = m.conversation_id AND assistant.reply_command_id = command_row.id AND assistant.role = ? AND assistant.is_del = ?", enum.AIMessageRoleAssistant, enum.CommonNo).
		Joins("JOIN ai_runs run_row ON run_row.user_message_id = m.id AND run_row.assistant_message_id = assistant.id AND run_row.conversation_id = m.conversation_id AND run_row.user_id = conversation.user_id").
		Where("m.role = ? AND m.is_del = ?", enum.AIMessageRoleUser, enum.CommonNo).
		Where(`((command_row.state = ? AND run_row.status = ? AND assistant.delivery_state = ?)
			OR (command_row.state = ? AND run_row.status = ? AND assistant.delivery_state = ?))`,
			replycommand.StateSucceeded, enum.AIRunStatusSuccess, replycommand.DeliveryStateCompleted,
			replycommand.StateCanceled, enum.AIRunStatusCanceled, replycommand.DeliveryStateStopped).
		Where(`NOT EXISTS (
			SELECT 1 FROM ai_tool_calls incomplete_tool
			WHERE incomplete_tool.run_id = run_row.id
			AND (incomplete_tool.call_id IS NULL OR incomplete_tool.call_id = '' OR incomplete_tool.status <> 'success' OR incomplete_tool.result_json IS NULL)
		)`)
}

func (repository *GormConversationRepository) buildTurns(
	ctx context.Context,
	rows []conversationTurnRow,
	conversationID uint64,
	userID uint64,
) ([]ConversationTurn, error) {
	runIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		runIDs = append(runIDs, row.RunID)
	}
	var tools []conversationToolRow
	toolQuery := repository.db.WithContext(ctx).Table("ai_tool_calls").
		Select("id, run_id, call_id, tool_code, status, arguments_json, result_json")
	if len(runIDs) == 0 {
		toolQuery = toolQuery.Where("run_id = 0")
	} else {
		toolQuery = toolQuery.Where("run_id IN ?", runIDs)
	}
	if err := toolQuery.Order("run_id ASC, id ASC").Scan(&tools).Error; err != nil {
		return nil, err
	}
	toolsByRun := make(map[uint64][]conversationToolRow, len(rows))
	for _, tool := range tools {
		toolsByRun[tool.RunID] = append(toolsByRun[tool.RunID], tool)
	}
	turns := make([]ConversationTurn, 0, len(rows))
	for _, row := range rows {
		if row.ConversationID != conversationID || row.ConversationUserID != userID ||
			row.RunUserID != userID || row.RunAgentID != row.ConversationAgent ||
			row.UserRole != enum.AIMessageRoleUser || row.AssistantRole != enum.AIMessageRoleAssistant ||
			row.AssistantDelivery == nil {
			return nil, fmt.Errorf("%w: inconsistent row for anchor %d", errTurnInvalid, row.UserMessageID)
		}
		attachments, err := turnAttachments(row.UserMetaJSON)
		if err != nil {
			continue
		}
		groups, complete := turnToolGroups(toolsByRun[row.RunID])
		if !complete {
			continue
		}
		turn := ConversationTurn{
			ConversationID: row.ConversationID, UserID: row.ConversationUserID, AgentID: row.ConversationAgent,
			UserMessage:       TurnMessage{ID: row.UserMessageID, Role: "user", Content: row.UserContent, ContentSHA256: sha256.Sum256([]byte(row.UserContent)), Attachments: attachments},
			ToolGroups:        groups,
			AssistantMessage:  TurnMessage{ID: row.AssistantMessageID, Role: "assistant", Content: row.AssistantContent, ContentSHA256: sha256.Sum256([]byte(row.AssistantContent))},
			AssistantDelivery: *row.AssistantDelivery,
		}
		if err := turn.ComputeSourceSHA256(); err != nil {
			continue
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func turnToolGroups(rows []conversationToolRow) ([]ToolGroup, bool) {
	groups := make([]ToolGroup, 0, len(rows))
	for _, row := range rows {
		if row.CallID == nil || strings.TrimSpace(*row.CallID) == "" || strings.TrimSpace(row.ToolCode) == "" ||
			row.Status != "success" || row.ResultJSON == nil || !json.Valid([]byte(row.ArgumentsJSON)) || !json.Valid([]byte(*row.ResultJSON)) {
			return nil, false
		}
		groups = append(groups, ToolGroup{CallID: *row.CallID, Name: row.ToolCode, Arguments: row.ArgumentsJSON, Result: *row.ResultJSON})
	}
	return groups, true
}

func turnAttachments(raw *string) ([]TurnAttachment, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	var meta struct {
		Attachments []struct {
			Type      string `json:"type"`
			ObjectKey string `json:"object_key"`
			MIMEType  string `json:"mime_type"`
			Name      string `json:"name"`
			Size      int64  `json:"size"`
			ETag      string `json:"etag"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(*raw), &meta); err != nil {
		return nil, err
	}
	attachments := make([]TurnAttachment, len(meta.Attachments))
	for i, attachment := range meta.Attachments {
		attachments[i] = TurnAttachment{
			Index: uint32(i), Type: strings.TrimSpace(attachment.Type), StorageProvider: "cos",
			ObjectKey: strings.TrimSpace(attachment.ObjectKey), ETag: strings.TrimSpace(attachment.ETag),
			Size: attachment.Size, MIMEType: strings.TrimSpace(attachment.MIMEType), Name: strings.TrimSpace(attachment.Name),
		}
	}
	return attachments, nil
}
