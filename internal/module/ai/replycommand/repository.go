package replycommand

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured = errors.New("reply command repository not configured")
	ErrConversationUnavailable = errors.New("owned active conversation not found")
	ErrCreateInputInvalid      = errors.New("reply command create input is invalid")
	ErrReplyCommandNotFound    = errors.New("reply command not found")
	errPublishLeaseLost        = errors.New("assistant publication lease lost")
)

const defaultMaxAttempts = 3

type Repository interface {
	CreateReply(context.Context, CreateReplyInput) (CreateReplyResult, error)
	RequestCancel(context.Context, int64, int64, string, time.Time) (*Command, error)
	ClaimNext(context.Context, string, time.Time, time.Duration) (*Claim, error)
	ClaimByID(context.Context, uint64, string, time.Time, time.Duration) (*Claim, error)
	Renew(context.Context, uint64, string, uint64, time.Time) (Renewal, error)
	Transition(context.Context, uint64, string, uint64, State, State, map[string]any) (bool, error)
	PublishAssistant(context.Context, PublishAssistantInput) (int64, bool, error)
}

func (r *GormRepository) RequestCancel(ctx context.Context, conversationID int64, userID int64, requestID string, now time.Time) (*Command, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	requestID = strings.TrimSpace(requestID)
	if conversationID <= 0 || userID <= 0 || requestID == "" {
		return nil, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now()
	}
	var command Command
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation replyConversation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
			First(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConversationUnavailable
		}
		if err != nil {
			return err
		}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("conversation_id = ? AND user_id = ? AND request_id = ?", conversationID, userID, requestID).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReplyCommandNotFound
		}
		if err != nil {
			return err
		}
		if terminalState(command.State) || command.State == StateOutcomeUnknown {
			return nil
		}

		updates := map[string]any{"cancel_requested_at": now, "updated_at": now}
		command.CancelRequestedAt = &now
		if command.State == StatePending {
			updates["state"] = StateCanceled
			updates["finished_at"] = now
			updates["lease_owner"] = nil
			updates["lease_expires_at"] = nil
			command.State = StateCanceled
			command.FinishedAt = &now
			command.LeaseOwner = nil
			command.LeaseExpiresAt = nil
		}
		if err := tx.Model(&Command{}).Where("id = ?", command.ID).Updates(updates).Error; err != nil {
			return err
		}
		command.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &command, nil
}

type Claim struct {
	Command        Command
	Owner          string
	FencingToken   uint64
	LeaseExpiresAt time.Time
}

type PublishAssistantInput struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Content   string
	Now       time.Time
}

type GormRepository struct {
	db             *gorm.DB
	now            func() time.Time
	idempotencyKey func(int64, string) string
}

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return newGormRepository(client.Gorm)
}

func newGormRepository(db *gorm.DB) *GormRepository {
	if db == nil {
		return nil
	}
	return &GormRepository{db: db, now: time.Now, idempotencyKey: idempotencyKey}
}

func (r *GormRepository) CreateReply(ctx context.Context, input CreateReplyInput) (CreateReplyResult, error) {
	if r == nil || r.db == nil {
		return CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.ConversationID <= 0 || input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 64 {
		return CreateReplyResult{}, fmt.Errorf("%w: conversation_id, user_id and request_id are required and request_id is at most 64 characters", ErrCreateInputInvalid)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	var result CreateReplyResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation replyConversation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			First(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConversationUnavailable
		}
		if err != nil {
			return err
		}

		var existing Command
		err = tx.Where("conversation_id = ? AND request_id = ?", input.ConversationID, input.RequestID).
			First(&existing).Error
		if err == nil {
			result = CreateReplyResult{
				UserMessageID: existing.UserMessageID,
				CommandID:     existing.ID,
				RequestID:     existing.RequestID,
				State:         existing.State,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		message := replyMessage{
			ConversationID: input.ConversationID,
			Role:           enum.AIMessageRoleUser,
			ContentType:    "text",
			Content:        input.Content,
			MetaJSON:       input.MetaJSON,
			IsDel:          enum.CommonNo,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}

		key := idempotencyKey(input.ConversationID, input.RequestID)
		if r.idempotencyKey != nil {
			key = r.idempotencyKey(input.ConversationID, input.RequestID)
		}
		command := Command{
			RequestID:      input.RequestID,
			IdempotencyKey: key,
			Platform:       enum.PlatformAdmin,
			UserID:         input.UserID,
			ConversationID: input.ConversationID,
			UserMessageID:  message.ID,
			State:          StatePending,
			MaxAttempts:    defaultMaxAttempts,
			NextAttemptAt:  now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&command).Error; err != nil {
			return err
		}

		updates := map[string]any{"last_message_at": now, "updated_at": now}
		if err := tx.Model(&replyConversation{}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			Updates(updates).Error; err != nil {
			return err
		}
		if title := titleFromContent(input.Content); title != "" {
			if err := tx.Model(&replyConversation{}).
				Where("id = ? AND user_id = ? AND is_del = ? AND title = ''", input.ConversationID, input.UserID, enum.CommonNo).
				Update("title", title).Error; err != nil {
				return err
			}
		}
		result = CreateReplyResult{
			UserMessageID: message.ID,
			CommandID:     command.ID,
			RequestID:     command.RequestID,
			State:         command.State,
		}
		return nil
	})
	if err != nil {
		return CreateReplyResult{}, err
	}
	return result, nil
}

func (r *GormRepository) ClaimNext(ctx context.Context, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	return r.claim(ctx, 0, owner, now, ttl)
}

func (r *GormRepository) ClaimByID(ctx context.Context, commandID uint64, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if commandID == 0 {
		return nil, ErrCreateInputInvalid
	}
	return r.claim(ctx, commandID, owner, now, ttl)
}

func (r *GormRepository) claim(ctx context.Context, commandID uint64, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if owner == "" || ttl <= 0 {
		return nil, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	leaseExpiresAt := now.Add(ttl)
	var claimed *Claim
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("cancel_requested_at IS NULL").
			Where("attempt_count < max_attempts").
			Where("(state = ? AND next_attempt_at <= ?) OR (state IN ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?)", StatePending, now, []State{StateClaimed, StateRunning}, now)
		if commandID > 0 {
			query = query.Where("id = ?", commandID)
		}
		var command Command
		err := query.Order("next_attempt_at ASC, id ASC").First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		nextToken := command.LeaseToken + 1
		updates := map[string]any{
			"state":            StateClaimed,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"lease_owner":      owner,
			"lease_token":      nextToken,
			"lease_expires_at": leaseExpiresAt,
			"updated_at":       now,
		}
		result := tx.Model(&Command{}).
			Where("id = ? AND lease_token = ?", command.ID, command.LeaseToken).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		command.State = StateClaimed
		command.AttemptCount++
		command.LeaseOwner = &owner
		command.LeaseToken = nextToken
		command.LeaseExpiresAt = &leaseExpiresAt
		command.UpdatedAt = now
		claimed = &Claim{Command: command, Owner: owner, FencingToken: nextToken, LeaseExpiresAt: leaseExpiresAt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *GormRepository) Renew(ctx context.Context, commandID uint64, owner string, token uint64, leaseExpiresAt time.Time) (Renewal, error) {
	if r == nil || r.db == nil {
		return Renewal{}, ErrRepositoryNotConfigured
	}
	if commandID == 0 || strings.TrimSpace(owner) == "" || token == 0 || leaseExpiresAt.IsZero() {
		return Renewal{}, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var renewal Renewal
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Command{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state IN ? AND cancel_requested_at IS NULL", commandID, owner, token, []State{StateClaimed, StateRunning}).
			Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			renewal.Alive = true
			return nil
		}
		var command Command
		err := tx.Select("id", "cancel_requested_at").
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state IN ?", commandID, owner, token, []State{StateClaimed, StateRunning}).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		renewal.Alive = true
		renewal.CancelRequested = command.CancelRequestedAt != nil
		return nil
	})
	return renewal, err
}

func (r *GormRepository) Transition(ctx context.Context, commandID uint64, owner string, token uint64, from State, to State, values map[string]any) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if commandID == 0 || strings.TrimSpace(owner) == "" || token == 0 || from == "" || to == "" {
		return false, ErrCreateInputInvalid
	}
	updates := make(map[string]any, len(values)+2)
	for key, value := range values {
		switch strings.TrimSpace(key) {
		case "id", "state", "lease_owner", "lease_token":
			continue
		default:
			updates[key] = value
		}
	}
	updates["state"] = to
	updates["updated_at"] = time.Now()
	if terminalState(to) || to == StatePending || to == StateOutcomeUnknown {
		updates["lease_owner"] = nil
		updates["lease_expires_at"] = nil
	}
	result := r.db.WithContext(ctx).Model(&Command{}).
		Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, from).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) PublishAssistant(ctx context.Context, input PublishAssistantInput) (int64, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, ErrRepositoryNotConfigured
	}
	input.Owner = strings.TrimSpace(input.Owner)
	if input.CommandID == 0 || input.Owner == "" || input.Token == 0 {
		return 0, false, ErrCreateInputInvalid
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	var assistantID int64
	var published bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing replyMessage
		err := tx.Where("reply_command_id = ?", input.CommandID).First(&existing).Error
		if err == nil {
			var command Command
			err = tx.Select("id", "lease_token", "state", "assistant_message_id").
				Where("id = ? AND lease_token = ? AND state = ? AND assistant_message_id = ?", input.CommandID, input.Token, StateSucceeded, existing.ID).
				First(&command).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			assistantID = existing.ID
			published = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var command Command
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL", input.CommandID, input.Owner, input.Token, StateRunning).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		replyCommandID := input.CommandID
		message := replyMessage{
			ConversationID: command.ConversationID,
			ReplyCommandID: &replyCommandID,
			Role:           enum.AIMessageRoleAssistant,
			ContentType:    "text",
			Content:        strings.TrimSpace(input.Content),
			IsDel:          enum.CommonNo,
			CreatedAt:      input.Now,
			UpdatedAt:      input.Now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&replyConversation{}).
			Where("id = ? AND user_id = ? AND is_del = ?", command.ConversationID, command.UserID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": input.Now, "updated_at": input.Now}).Error; err != nil {
			return err
		}
		result := tx.Model(&Command{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL", input.CommandID, input.Owner, input.Token, StateRunning).
			Updates(map[string]any{
				"state":                StateSucceeded,
				"assistant_message_id": message.ID,
				"finished_at":          input.Now,
				"lease_owner":          nil,
				"lease_expires_at":     nil,
				"updated_at":           input.Now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errPublishLeaseLost
		}
		assistantID = message.ID
		published = true
		return nil
	})
	if errors.Is(err, errPublishLeaseLost) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return assistantID, published, nil
}

func terminalState(state State) bool {
	switch state {
	case StateSucceeded, StateFailed, StateCanceled, StateTimedOut:
		return true
	default:
		return false
	}
}

func idempotencyKey(conversationID int64, requestID string) string {
	sum := sha256.Sum256([]byte("admin-reply:" + strconv.FormatInt(conversationID, 10) + ":" + strings.TrimSpace(requestID)))
	return hex.EncodeToString(sum[:])
}

func titleFromContent(content string) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) > 30 {
		return string(runes[:30])
	}
	return content
}

type replyConversation struct {
	ID      int64  `gorm:"column:id;primaryKey"`
	UserID  int64  `gorm:"column:user_id"`
	AgentID int64  `gorm:"column:agent_id"`
	Title   string `gorm:"column:title"`
	IsDel   int    `gorm:"column:is_del"`
}

func (replyConversation) TableName() string { return "ai_conversations" }

type replyMessage struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	ConversationID int64     `gorm:"column:conversation_id"`
	ReplyCommandID *uint64   `gorm:"column:reply_command_id"`
	Role           int       `gorm:"column:role"`
	ContentType    string    `gorm:"column:content_type"`
	Content        string    `gorm:"column:content"`
	MetaJSON       *string   `gorm:"column:meta_json"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (replyMessage) TableName() string { return "ai_messages" }
