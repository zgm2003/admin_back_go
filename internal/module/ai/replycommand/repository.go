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
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRepositoryNotConfigured    = errors.New("reply command repository not configured")
	ErrConversationUnavailable    = errors.New("owned active conversation not found")
	ErrCreateInputInvalid         = errors.New("reply command create input is invalid")
	ErrUploadRuleChanged          = errors.New("reply command upload rule changed")
	ErrReplyCommandNotFound       = errors.New("reply command not found")
	ErrAttemptNotFound            = errors.New("provider attempt not found")
	ErrAttemptTransactionRequired = errors.New("provider attempt requires an active outer transaction")
	ErrAttemptTerminalConflict    = errors.New("provider attempt terminal evidence conflicts with persisted evidence")
	errPublishLeaseLost           = errors.New("assistant publication lease lost")
)

const defaultMaxAttempts = 3

type Repository interface {
	CreateReply(context.Context, CreateReplyInput) (CreateReplyResult, error)
	RequestCancel(context.Context, RequestCancelInput) (RequestCancelResult, error)
	ClaimNext(context.Context, ClaimSource, string, time.Time, time.Duration) (*Claim, error)
	ClaimByID(context.Context, uint64, ClaimSource, string, time.Time, time.Duration) (*Claim, error)
	Renew(context.Context, uint64, string, uint64, time.Time) (Renewal, error)
	Transition(context.Context, uint64, string, uint64, State, State, map[string]any) (bool, error)
	PublishAssistant(context.Context, PublishAssistantInput) (int64, bool, error)
	PrepareLegacyAttempt(context.Context, LegacyPrepareAttemptInput) (*Attempt, bool, error)
	MarkAttemptDispatched(context.Context, uint64, uint64, string, uint64, time.Time) (bool, error)
	FinishAttempt(context.Context, FinishAttemptInput) (bool, error)
}

type UploadRuleTransactionGuard interface {
	GuardActiveInTransaction(context.Context, *gorm.DB, uploadpolicy.ConsistencyToken) error
}

type UploadRuleGuardedRepository interface {
	CreateReplyWithUploadRuleGuard(context.Context, CreateReplyInput, UploadRuleTransactionGuard) (CreateReplyResult, error)
}

func (r *GormRepository) RequestCancel(ctx context.Context, input RequestCancelInput) (RequestCancelResult, error) {
	if r == nil || r.db == nil {
		return RequestCancelResult{}, ErrRepositoryNotConfigured
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.ConversationID <= 0 || input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 {
		return RequestCancelResult{}, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	var result RequestCancelResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND request_id = ? AND conversation_id = ?", input.UserID, input.RequestID, input.ConversationID).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReplyCommandNotFound
		}
		if err != nil {
			return err
		}
		result.CommandID = command.ID
		if terminalState(command.State) || command.State == StateOutcomeUnknown {
			result.Status = CancelStatusAlreadyTerminal
			if command.AssistantMessageID != nil {
				result.AssistantMessageID = *command.AssistantMessageID
			}
			return nil
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
		if command.CancelRequestedAt != nil {
			result.Status = CancelStatusStopped
			result.SettlementPending = settlementPendingState(command.State)
			result.DeliveryConsistent = true
			if command.AssistantMessageID != nil {
				result.AssistantMessageID = *command.AssistantMessageID
			}
			if command.StopDeliverySeq != nil {
				result.StopDeliverySeq = *command.StopDeliverySeq
			}
			return nil
		}

		prefix, err := r.ReadDeliveryPrefixTx(ctx, tx, command.ID, input.DeliveredSeq)
		if err != nil {
			return err
		}
		if input.DeliveredSeq > command.DeliverySeq {
			prefix = DeliveryPrefix{}
		}
		stopDeliverySeq := prefix.StopDeliverySeq
		content := prefix.Content
		if !prefix.Consistent {
			stopDeliverySeq = 0
			content = ""
		}
		replyCommandID := command.ID
		deliveryState := DeliveryStateStopped
		message := replyMessage{
			ConversationID: input.ConversationID,
			ReplyCommandID: &replyCommandID,
			Role:           enum.AIMessageRoleAssistant,
			ContentType:    "text",
			Content:        content,
			DeliveryState:  &deliveryState,
			IsDel:          enum.CommonNo,
			CreatedAt:      input.Now,
			UpdatedAt:      input.Now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&replyConversation{}).
			Where("id = ? AND user_id = ? AND is_del = ?", input.ConversationID, input.UserID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": input.Now, "updated_at": input.Now}).Error; err != nil {
			return err
		}
		updated := tx.Model(&Command{}).Where("id = ?", command.ID).Updates(map[string]any{
			"cancel_requested_at":  input.Now,
			"stop_delivery_seq":    stopDeliverySeq,
			"assistant_message_id": message.ID,
			"updated_at":           input.Now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrReplyCommandNotFound
		}
		result.Status = CancelStatusStopped
		result.AssistantMessageID = message.ID
		result.SettlementPending = settlementPendingState(command.State)
		result.DeliveryConsistent = prefix.Consistent
		result.StopDeliverySeq = stopDeliverySeq
		return nil
	})
	if err != nil {
		return RequestCancelResult{}, err
	}
	if result.Status == CancelStatusStopped && result.CommandID > 0 {
		_ = CleanupDeliveryChunks(context.WithoutCancel(ctx), r, result.CommandID, 1)
	}
	return result, nil
}

func settlementPendingState(state State) bool {
	switch state {
	case StatePending, StateClaimed, StateRunning:
		return true
	default:
		return false
	}
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
	return r.createReply(ctx, input, nil)
}

func (r *GormRepository) CreateReplyWithUploadRuleGuard(ctx context.Context, input CreateReplyInput, guard UploadRuleTransactionGuard) (CreateReplyResult, error) {
	return r.createReply(ctx, input, guard)
}

func (r *GormRepository) createReply(ctx context.Context, input CreateReplyInput, guard UploadRuleTransactionGuard) (CreateReplyResult, error) {
	if r == nil || r.db == nil {
		return CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ModelDisplayName = strings.TrimSpace(input.ModelDisplayName)
	input.InputSnapshot = strings.TrimSpace(input.InputSnapshot)
	pricingSnapshot, pricingErr := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
	if input.ConversationID <= 0 || input.UserID <= 0 || input.AgentID <= 0 || input.ProviderID <= 0 || input.ModelID == "" || input.InputSnapshot == "" || input.RequestID == "" || input.RequestReceivedAt.IsZero() || utf8.RuneCountInString(input.RequestID) > 128 || input.RequestFingerprint == ([32]byte{}) || input.RequestIdentityStatus != requestidentity.IdentityStatusReplayable || input.RequestIdentityMarker != "" || pricingErr != nil || pricingSnapshot.RequestedModelID != input.ModelID || int64(pricingSnapshot.EffectiveMaxOutputTokens) != input.EffectiveMaxTokens {
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
			accepted, err := loadAcceptedRunCharge(tx, input.UserID, input.RequestID, true)
			if err != nil {
				return err
			}
			result = CreateReplyResult{
				UserMessageID: existing.UserMessageID,
				CommandID:     existing.ID,
				RunID:         accepted.RunID,
				ChargeID:      accepted.ChargeID,
				RequestID:     existing.RequestID,
				State:         existing.State,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.UploadRuleToken != (uploadpolicy.ConsistencyToken{}) {
			if guard == nil {
				return ErrUploadRuleChanged
			}
			if guardErr := guard.GuardActiveInTransaction(ctx, tx, input.UploadRuleToken); guardErr != nil {
				if errors.Is(guardErr, uploadpolicy.ErrRuleSnapshotChanged) {
					return ErrUploadRuleChanged
				}
				return guardErr
			}
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
			RequestReceivedAt:     &input.RequestReceivedAt,
			AcceptedAt:            &now,
			State:                 StatePending,
			MaxAttempts:           defaultMaxAttempts,
			NextAttemptAt:         now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := tx.Create(&command).Error; err != nil {
			return err
		}
		conversationID := input.ConversationID
		userMessageID := message.ID
		idempotency := key
		run := airun.Run{
			Platform: enum.PlatformAdmin, ConversationID: &conversationID, RequestID: input.RequestID,
			RequestFingerprint: append([]byte(nil), input.RequestFingerprint[:]...), RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RequestIdentityMarker: "",
			IdempotencyKey: &idempotency, UserMessageID: &userMessageID, UserID: input.UserID, AgentID: input.AgentID, ProviderID: input.ProviderID,
			ModelID: input.ModelID, ModelDisplayName: input.ModelDisplayName, InputSnapshot: input.InputSnapshot, PricingSnapshotJSON: strings.TrimSpace(input.PricingSnapshotJSON),
			Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusPending), BillingReason: string(billing.BillingReasonPending), StartedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Create(&airun.RunEvent{RunID: run.ID, Seq: 1, EventType: enum.AIRunEventStart, Message: enum.AIRunEventLabels[enum.AIRunEventStart], CreatedAt: now}).Error; err != nil {
			return err
		}
		charge := billing.UsageCharge{
			RunID: run.ID, UserID: input.UserID, Currency: "CNY", PricingVersion: pricingSnapshot.Version, MultiplierPPM: pricingSnapshot.MultiplierPPM,
			Status: billing.ChargeStatusOpen, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&charge).Error; err != nil {
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
			RunID:         run.ID,
			ChargeID:      charge.ID,
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

func loadAcceptedRunCharge(db *gorm.DB, userID int64, requestID string, lock bool) (billing.AcceptedRun, error) {
	query := db.Where("user_id = ? AND request_id = ?", userID, requestID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run airun.Run
	if err := query.Select("id").First(&run).Error; err != nil {
		return billing.AcceptedRun{}, err
	}
	chargeQuery := db.Where("run_id = ?", run.ID)
	if lock {
		chargeQuery = chargeQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var charge billing.UsageCharge
	if err := chargeQuery.Select("id").First(&charge).Error; err != nil {
		return billing.AcceptedRun{}, err
	}
	return billing.AcceptedRun{RunID: run.ID, ChargeID: charge.ID}, nil
}

func (r *GormRepository) ClaimNext(ctx context.Context, source ClaimSource, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	return r.claim(ctx, 0, source, owner, now, ttl)
}

func (r *GormRepository) ClaimByID(ctx context.Context, commandID uint64, source ClaimSource, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if commandID == 0 {
		return nil, ErrCreateInputInvalid
	}
	return r.claim(ctx, commandID, source, owner, now, ttl)
}

func (r *GormRepository) claim(ctx context.Context, commandID uint64, source ClaimSource, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if owner == "" || ttl <= 0 || (source != ClaimSourceWake && source != ClaimSourcePoll) {
		return nil, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	leaseExpiresAt := now.Add(ttl)
	var claimed *Claim
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("attempt_count < max_attempts OR cancel_requested_at IS NOT NULL OR ((last_error_code = ? AND last_error_message = ?) OR (last_error_code = ? AND last_error_message = ?) OR (last_error_code = ? AND last_error_message = ?) OR (last_error_code = ? AND last_error_message = ?) OR (last_error_code = ? AND last_error_message IN ?)) OR EXISTS (SELECT 1 FROM ai_provider_attempts pa WHERE pa.command_id = ai_reply_commands.id AND pa.state = ?) OR (state IN ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?)", ErrCodeFinalizationRetry, FinalizationRetryMarker, "ai.provider_failed", "provider_failed", "ai.local_failed", "local_failure", "ai.provider_pre_dispatch_failed", "pre_dispatch_failed", "ai.billing.insufficient_balance", []string{"initial_insufficient", "continuation_topup_insufficient"}, AttemptPrepared, []State{StateClaimed, StateRunning}, now).
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
		skipAttemptIncrement := command.RequiresFinalizationOnly()
		// Always inspect the latest durable attempt: a prepared request must be
		// replayed without consuming another provider attempt even below the cap.
		{
			var latestAttempt Attempt
			attemptErr := tx.Where("command_id = ?", command.ID).Order("attempt_no DESC").First(&latestAttempt).Error
			if attemptErr != nil && !errors.Is(attemptErr, gorm.ErrRecordNotFound) {
				return attemptErr
			}
			if (command.State == StateClaimed || command.State == StateRunning) && attemptErr == nil && ambiguousAttemptState(latestAttempt.State) {
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
			if attemptErr == nil {
				switch latestAttempt.State {
				case AttemptSucceeded:
					// Runner's Finalizer probe decides whether this is a final answer,
					// unbilled evidence, or a tool continuation. Do not overwrite the
					// candidate with a generic finalization marker here.
					skipAttemptIncrement = true
				case AttemptPrepared:
					// The exact prepared request is replayed; it is not another provider attempt.
					skipAttemptIncrement = true
				}
			}
		}
		nextToken := command.LeaseToken + 1
		updates := map[string]any{
			"state":            StateClaimed,
			"lease_owner":      owner,
			"lease_token":      nextToken,
			"lease_expires_at": leaseExpiresAt,
			"claimed_at":       now,
			"claim_source":     source,
			"updated_at":       now,
		}
		if !skipAttemptIncrement {
			updates["attempt_count"] = gorm.Expr("attempt_count + 1")
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
		if !skipAttemptIncrement {
			command.AttemptCount++
		}
		command.LeaseOwner = &owner
		command.LeaseToken = nextToken
		command.LeaseExpiresAt = &leaseExpiresAt
		command.ClaimedAt = &now
		command.ClaimSource = source
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

func (r *GormRepository) ScheduleRetry(ctx context.Context, commandID uint64, owner string, token uint64, now time.Time, next time.Time, errorCode string, errorMessage string) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	errorCode = strings.TrimSpace(errorCode)
	errorMessage = strings.TrimSpace(errorMessage)
	if commandID == 0 || owner == "" || token == 0 || next.IsZero() || errorCode == "" || errorMessage == "" {
		return false, ErrCreateInputInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}
	var applied bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, StateRunning).First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{"state": StatePending, "next_attempt_at": next, "last_error_code": errorCode, "last_error_message": errorMessage, "lease_owner": nil, "lease_expires_at": nil, "updated_at": now}
		if err := tx.Model(&Command{}).Where("id = ? AND state = ? AND lease_owner = ? AND lease_token = ?", commandID, StateRunning, owner, token).Updates(updates).Error; err != nil {
			return err
		}
		var run airun.Run
		if err := tx.Where("user_id = ? AND request_id = ?", command.UserID, command.RequestID).First(&run).Error; err != nil {
			return err
		}
		var maxSeq uint
		if err := tx.Model(&airun.RunEvent{}).Where("run_id = ?", run.ID).Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		if err := tx.Create(&airun.RunEvent{RunID: run.ID, Seq: maxSeq + 1, EventType: enum.AIRunEventRetryScheduled, Message: enum.AIRunEventLabels[enum.AIRunEventRetryScheduled], CreatedAt: now}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

// ScheduleFinalizationRetry preserves the durable terminal marker. The next
// claim must re-enter settlement rather than issue a fresh provider request.
func (r *GormRepository) ScheduleFinalizationRetry(ctx context.Context, commandID uint64, owner string, token uint64, now time.Time, next time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if commandID == 0 || owner == "" || token == 0 || next.IsZero() {
		return false, ErrCreateInputInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}
	var applied bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, StateRunning).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		marker := command
		marker.CancelRequestedAt = nil
		updates := map[string]any{"state": StatePending, "next_attempt_at": next, "lease_owner": nil, "lease_expires_at": nil, "updated_at": now}
		if !marker.RequiresFinalizationOnly() {
			updates["last_error_code"] = ErrCodeFinalizationRetry
			updates["last_error_message"] = FinalizationRetryMarker
		}
		result := tx.Model(&Command{}).
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ?", commandID, owner, token, StateRunning).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		applied = result.RowsAffected == 1
		return nil
	})
	return applied, err
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
			payload, payloadErr := canceledTerminalPayload(command)
			if payloadErr != nil {
				return payloadErr
			}
			eventInput.Type = modulerealtime.TypeAIResponseCanceledV2
			eventInput.Payload = payload
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

func canceledTerminalPayload(command Command) (modulerealtime.AIResponseCanceledPayload, error) {
	if command.ConversationID <= 0 || strings.TrimSpace(command.RequestID) == "" || command.AssistantMessageID == nil || *command.AssistantMessageID <= 0 {
		return modulerealtime.AIResponseCanceledPayload{}, errors.New("canceled terminal event requires stopped assistant message")
	}
	return modulerealtime.AIResponseCanceledPayload{
		ConversationID:     command.ConversationID,
		RequestID:          command.RequestID,
		AssistantMessageID: *command.AssistantMessageID,
	}, nil
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
			DeliveryState:  stringPointer(DeliveryStateCompleted),
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
	accepted, err := loadAcceptedRunCharge(r.db.WithContext(ctx), input.UserID, input.RequestID, false)
	if err != nil {
		return nil, err
	}
	return &CreateReplyResult{UserMessageID: command.UserMessageID, CommandID: command.ID, RunID: accepted.RunID, ChargeID: accepted.ChargeID, RequestID: command.RequestID, State: command.State}, nil
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
	DeliveryState  *string   `gorm:"column:delivery_state"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func stringPointer(value string) *string { return &value }

func (replyMessage) TableName() string { return "ai_messages" }
