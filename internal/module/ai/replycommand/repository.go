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
	"admin_back_go/internal/module/ai/requestidentity"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured    = errors.New("reply command repository not configured")
	ErrConversationUnavailable    = errors.New("owned active conversation not found")
	ErrCreateInputInvalid         = errors.New("reply command create input is invalid")
	ErrReplyCommandNotFound       = errors.New("reply command not found")
	ErrAttemptNotFound            = errors.New("provider attempt not found")
	ErrAttemptTransactionRequired = errors.New("provider attempt requires an active outer transaction")
	ErrAttemptTerminalConflict    = errors.New("provider attempt terminal evidence conflicts with persisted evidence")
	errPublishLeaseLost           = errors.New("assistant publication lease lost")
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
	PrepareLegacyAttempt(context.Context, LegacyPrepareAttemptInput) (*Attempt, bool, error)
	MarkAttemptDispatched(context.Context, uint64, uint64, string, uint64, time.Time) (bool, error)
	FinishAttempt(context.Context, FinishAttemptInput) (bool, error)
}

func (r *GormRepository) RequestCancel(ctx context.Context, conversationID int64, userID int64, requestID string, now time.Time) (*Command, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	requestID = strings.TrimSpace(requestID)
	if conversationID <= 0 || userID <= 0 || requestID == "" || utf8.RuneCountInString(requestID) > 128 {
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
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND request_id = ?", userID, requestID).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReplyCommandNotFound
		}
		if err != nil {
			return err
		}
		if command.ConversationID != conversationID {
			return ErrReplyCommandNotFound
		}

		var conversation replyConversation
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND is_del = ?", conversationID, userID, enum.CommonNo).
			First(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConversationUnavailable
		}
		if err != nil {
			return err
		}
		if terminalState(command.State) || command.State == StateOutcomeUnknown || command.CancelRequestedAt != nil {
			return nil
		}

		updates := map[string]any{"cancel_requested_at": now, "updated_at": now}
		command.CancelRequestedAt = &now
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
	eventSink      modulerealtime.TransactionalEventSink
}

type RepositoryOption func(*GormRepository)

func WithDurableEventSink(sink modulerealtime.TransactionalEventSink) RepositoryOption {
	return func(repository *GormRepository) {
		repository.eventSink = sink
	}
}

func NewGormRepository(client *database.Client, options ...RepositoryOption) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	repository := newGormRepository(client.Gorm)
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
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
	if input.ConversationID <= 0 || input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 || input.RequestFingerprint == ([32]byte{}) || input.RequestIdentityStatus != requestidentity.IdentityStatusReplayable || input.RequestIdentityMarker != "" {
		return CreateReplyResult{}, fmt.Errorf("%w: conversation_id, user_id and request_id are required and request_id is at most 128 characters", ErrCreateInputInvalid)
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
		var existing Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND request_id = ?", input.UserID, input.RequestID).
			First(&existing).Error
		if err == nil {
			if err := compareCommandFingerprint(existing, input.RequestFingerprint); err != nil {
				return err
			}
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

		var conversation replyConversation
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			First(&conversation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConversationUnavailable
		}
		if err != nil {
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

		key := idempotencyKey(input.UserID, input.RequestID)
		if r.idempotencyKey != nil {
			key = r.idempotencyKey(input.UserID, input.RequestID)
		}
		command := Command{
			RequestID:             input.RequestID,
			RequestFingerprint:    append([]byte(nil), input.RequestFingerprint[:]...),
			RequestIdentityStatus: string(input.RequestIdentityStatus),
			RequestIdentityMarker: input.RequestIdentityMarker,
			IdempotencyKey:        key,
			Platform:              enum.PlatformAdmin,
			UserID:                input.UserID,
			ConversationID:        input.ConversationID,
			UserMessageID:         message.ID,
			State:                 StatePending,
			MaxAttempts:           defaultMaxAttempts,
			NextAttemptAt:         now,
			CreatedAt:             now,
			UpdatedAt:             now,
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
		if replay, replayErr := r.loadCreateReplay(ctx, input); replayErr == nil && replay != nil {
			return *replay, nil
		} else if replayErr != nil {
			return CreateReplyResult{}, replayErr
		}
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
		if command.State == StateClaimed || command.State == StateRunning {
			var latestAttempt Attempt
			attemptErr := tx.Where("command_id = ?", command.ID).Order("attempt_no DESC").First(&latestAttempt).Error
			if attemptErr != nil && !errors.Is(attemptErr, gorm.ErrRecordNotFound) {
				return attemptErr
			}
			if attemptErr == nil && ambiguousAttemptState(latestAttempt.State) {
				result := tx.Model(&Command{}).
					Where("id = ? AND lease_token = ? AND state IN ?", command.ID, command.LeaseToken, []State{StateClaimed, StateRunning}).
					Updates(map[string]any{
						"state":              StateOutcomeUnknown,
						"outcome_unknown_at": now,
						"last_error_code":    "ai.provider_outcome_unknown",
						"last_error_message": "expired worker left an acknowledged provider attempt",
						"lease_owner":        nil,
						"lease_expires_at":   nil,
						"updated_at":         now,
					})
				if result.Error != nil {
					return result.Error
				}
				return nil
			}
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
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state IN ?", commandID, owner, token, []State{StateClaimed, StateRunning}).
			Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
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
	if terminalState(to) {
		finishedAt, ok := updates["finished_at"].(time.Time)
		if !ok || finishedAt.IsZero() {
			return false, ErrCreateInputInvalid
		}
	}
	if to == StateFailed || to == StateTimedOut {
		code, ok := updates["last_error_code"].(string)
		if !ok || strings.TrimSpace(code) == "" || code != strings.TrimSpace(code) {
			return false, ErrCreateInputInvalid
		}
		message, ok := updates["last_error_message"].(string)
		if !ok || strings.TrimSpace(message) == "" {
			return false, ErrCreateInputInvalid
		}
	}
	if r.eventSink != nil && (to == StateFailed || to == StateTimedOut || to == StateCanceled) {
		return r.transitionWithTerminalEvent(ctx, commandID, strings.TrimSpace(owner), token, from, to, updates)
	}
	result := r.db.WithContext(ctx).Model(&Command{}).
		Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, from).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) transitionWithTerminalEvent(ctx context.Context, commandID uint64, owner string, token uint64, from State, to State, updates map[string]any) (bool, error) {
	var applied bool
	var durableEvent *modulerealtime.Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, from).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result := tx.Model(&Command{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, from).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		occurredAt := updates["finished_at"].(time.Time)
		eventInput := modulerealtime.AppendInput{
			RequestID:  command.RequestID,
			UserID:     command.UserID,
			OccurredAt: occurredAt,
		}
		if to == StateCanceled {
			eventInput.Type = modulerealtime.TypeAIResponseCanceledV1
			eventInput.Payload = modulerealtime.AIResponseCanceledPayload{
				ConversationID: command.ConversationID,
				RequestID:      command.RequestID,
			}
		} else {
			message, _ := updates["last_error_message"].(string)
			errorCode, _ := updates["last_error_code"].(string)
			var walletPath *string
			var rechargePath *string
			if strings.TrimSpace(errorCode) == "ai.billing.insufficient_balance" {
				wallet := "/profile/wallet"
				recharge := "/payment/recharge"
				walletPath = &wallet
				rechargePath = &recharge
			}
			eventInput.Type = modulerealtime.TypeAIResponseFailedV1
			eventInput.Payload = modulerealtime.AIResponseFailedPayload{
				ConversationID: command.ConversationID,
				RequestID:      command.RequestID,
				Msg:            strings.TrimSpace(message),
				ErrorCode:      strings.TrimSpace(errorCode),
				WalletPath:     walletPath,
				RechargePath:   rechargePath,
			}
		}
		durableEvent, err = r.eventSink.AppendTx(ctx, tx, eventInput)
		if err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if durableEvent != nil {
		r.eventSink.PublishBestEffort(ctx, durableEvent)
	}
	return applied, nil
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
	var durableEvent *modulerealtime.Event
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
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL AND lease_expires_at > ?", input.CommandID, input.Owner, input.Token, StateRunning, input.Now).
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
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL AND lease_expires_at > ?", input.CommandID, input.Owner, input.Token, StateRunning, input.Now).
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
		if r.eventSink != nil {
			durableEvent, err = r.eventSink.AppendTx(ctx, tx, modulerealtime.AppendInput{
				Type:      modulerealtime.TypeAIResponseCompletedV1,
				RequestID: command.RequestID,
				UserID:    command.UserID,
				Payload: modulerealtime.AIResponseCompletedPayload{
					ConversationID: command.ConversationID, RequestID: command.RequestID, AssistantMessageID: message.ID,
				},
				OccurredAt: input.Now,
			})
			if err != nil {
				return err
			}
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
	if durableEvent != nil {
		r.eventSink.PublishBestEffort(ctx, durableEvent)
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

func idempotencyKey(userID int64, requestID string) string {
	sum := sha256.Sum256([]byte("admin-reply:" + strconv.FormatInt(userID, 10) + ":" + strings.TrimSpace(requestID)))
	return hex.EncodeToString(sum[:])
}

func compareCommandFingerprint(command Command, incoming [32]byte) error {
	if len(command.RequestFingerprint) != sha256.Size {
		return requestidentity.ErrRequestIdentityNotReplayable
	}
	var stored [sha256.Size]byte
	copy(stored[:], command.RequestFingerprint)
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(command.RequestIdentityStatus), stored, incoming)
}

func (r *GormRepository) loadCreateReplay(ctx context.Context, input CreateReplyInput) (*CreateReplyResult, error) {
	var command Command
	err := r.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", input.UserID, input.RequestID).First(&command).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := compareCommandFingerprint(command, input.RequestFingerprint); err != nil {
		return nil, err
	}
	return &CreateReplyResult{UserMessageID: command.UserMessageID, CommandID: command.ID, RequestID: command.RequestID, State: command.State}, nil
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
