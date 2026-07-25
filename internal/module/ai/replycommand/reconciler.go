package replycommand

import (
	"context"
	"errors"
	"strings"
	"time"

	modulerealtime "admin_back_go/internal/module/realtime"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrReconcilerNotReady = errors.New("reply command reconciler is not ready")

type OutcomeUnknownWork struct {
	CommandID          uint64
	AssistantMessageID int64
	ProviderRequestID  string
}

type ReconcileOutcomeInput struct {
	CommandID          uint64
	State              State
	AssistantMessageID int64
	Content            string
	ErrorCode          string
	ErrorMessage       string
	Now                time.Time
}

type OutcomeRepository interface {
	NextOutcomeUnknown(context.Context) (*OutcomeUnknownWork, error)
	ResolveOutcomeUnknown(context.Context, ReconcileOutcomeInput) (bool, error)
}

type ProviderLookupStatus string

const (
	ProviderLookupFound    ProviderLookupStatus = "found"
	ProviderLookupRejected ProviderLookupStatus = "rejected"
	ProviderLookupUnknown  ProviderLookupStatus = "unknown"
)

type ProviderLookupResult struct {
	Status  ProviderLookupStatus
	Content string
}

type ProviderLookup interface {
	Lookup(context.Context, string) (ProviderLookupResult, error)
}

type ReconcilerOptions struct {
	Repository OutcomeRepository
	Lookup     ProviderLookup
	Now        func() time.Time
}

type Reconciler struct {
	repository OutcomeRepository
	lookup     ProviderLookup
	now        func() time.Time
}

func NewReconciler(options ReconcilerOptions) *Reconciler {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Reconciler{repository: options.Repository, lookup: options.Lookup, now: now}
}

func (r *Reconciler) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.repository == nil || r.now == nil {
		return false, ErrReconcilerNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := r.repository.NextOutcomeUnknown(ctx)
	if err != nil || work == nil {
		return false, err
	}
	resolution := ReconcileOutcomeInput{CommandID: work.CommandID, Now: r.now()}
	if work.AssistantMessageID > 0 {
		resolution.State = StateSucceeded
		resolution.AssistantMessageID = work.AssistantMessageID
	} else if r.lookup != nil && strings.TrimSpace(work.ProviderRequestID) != "" {
		result, lookupErr := r.lookup.Lookup(ctx, strings.TrimSpace(work.ProviderRequestID))
		if lookupErr != nil {
			return true, lookupErr
		}
		switch result.Status {
		case ProviderLookupFound:
			if strings.TrimSpace(result.Content) != "" {
				resolution.State = StateSucceeded
				resolution.Content = strings.TrimSpace(result.Content)
			} else {
				resolution = failedUnknownResolution(work.CommandID, r.now())
			}
		case ProviderLookupRejected:
			resolution.State = StateFailed
			resolution.ErrorCode = "ai.provider_rejected"
			resolution.ErrorMessage = "provider rejected the acknowledged attempt"
		default:
			resolution = failedUnknownResolution(work.CommandID, r.now())
		}
	} else {
		resolution = failedUnknownResolution(work.CommandID, r.now())
	}
	_, err = r.repository.ResolveOutcomeUnknown(ctx, resolution)
	return true, err
}

func failedUnknownResolution(commandID uint64, now time.Time) ReconcileOutcomeInput {
	return ReconcileOutcomeInput{
		CommandID:    commandID,
		State:        StateFailed,
		ErrorCode:    "ai.provider_outcome_unknown",
		ErrorMessage: "provider outcome cannot be queried; explicit operator retry requires a new request",
		Now:          now,
	}
}

func (r *GormRepository) NextOutcomeUnknown(ctx context.Context) (*OutcomeUnknownWork, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var command Command
	err := r.db.WithContext(ctx).Where("state = ?", StateOutcomeUnknown).Order("outcome_unknown_at ASC, id ASC").First(&command).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	work := &OutcomeUnknownWork{CommandID: command.ID}
	if command.AssistantMessageID != nil {
		work.AssistantMessageID = *command.AssistantMessageID
	} else {
		var message replyMessage
		err = r.db.WithContext(ctx).Select("id").Where("reply_command_id = ?", command.ID).First(&message).Error
		if err == nil {
			work.AssistantMessageID = message.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	var attempt Attempt
	err = r.db.WithContext(ctx).
		Where("command_id = ? AND state IN ?", command.ID, []AttemptState{AttemptDispatched, AttemptSucceeded, AttemptOutcomeUnknown}).
		Order("attempt_no DESC").
		First(&attempt).Error
	if err == nil {
		work.ProviderRequestID = attempt.ProviderRequestID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return work, nil
}

func (r *GormRepository) ResolveOutcomeUnknown(ctx context.Context, input ReconcileOutcomeInput) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if input.CommandID == 0 || (input.State != StateSucceeded && input.State != StateFailed) {
		return false, ErrCreateInputInvalid
	}
	if input.State == StateFailed && strings.TrimSpace(input.ErrorMessage) == "" {
		return false, ErrCreateInputInvalid
	}
	if input.State == StateFailed && strings.TrimSpace(input.ErrorCode) == "" {
		return false, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	resolved := false
	var durableEvent *modulerealtime.Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND state = ?", input.CommandID, StateOutcomeUnknown).First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var assistant replyMessage
		err = tx.Where("reply_command_id = ?", input.CommandID).First(&assistant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && input.AssistantMessageID > 0 {
			err = tx.Where("id = ? AND reply_command_id = ?", input.AssistantMessageID, input.CommandID).First(&assistant).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) && input.State == StateSucceeded && strings.TrimSpace(input.Content) != "" {
			commandID := input.CommandID
			assistant = replyMessage{
				ConversationID: command.ConversationID,
				ReplyCommandID: &commandID,
				Role:           enum.AIMessageRoleAssistant,
				ContentType:    "text",
				Content:        strings.TrimSpace(input.Content),
				IsDel:          enum.CommonNo,
				CreatedAt:      input.Now,
				UpdatedAt:      input.Now,
			}
			err = tx.Create(&assistant).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.State == StateSucceeded && assistant.ID == 0 {
			return ErrResultInvalid
		}
		updates := map[string]any{
			"state":              input.State,
			"finished_at":        input.Now,
			"outcome_unknown_at": nil,
			"lease_owner":        nil,
			"lease_expires_at":   nil,
			"updated_at":         input.Now,
		}
		if input.State == StateSucceeded {
			updates["assistant_message_id"] = assistant.ID
			updates["last_error_code"] = ""
			updates["last_error_message"] = ""
			if err := tx.Model(&replyConversation{}).
				Where("id = ? AND user_id = ? AND is_del = ?", command.ConversationID, command.UserID, enum.CommonNo).
				Updates(map[string]any{"last_message_at": input.Now, "updated_at": input.Now}).Error; err != nil {
				return err
			}
		} else {
			updates["last_error_code"] = strings.TrimSpace(input.ErrorCode)
			updates["last_error_message"] = strings.TrimSpace(input.ErrorMessage)
		}
		result := tx.Model(&Command{}).Where("id = ? AND state = ?", input.CommandID, StateOutcomeUnknown).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if r.eventSink != nil {
			eventInput := modulerealtime.AppendInput{RequestID: command.RequestID, UserID: command.UserID, OccurredAt: input.Now}
			if input.State == StateSucceeded {
				eventInput.Type = modulerealtime.TypeAIResponseCompletedV1
				eventInput.Payload = modulerealtime.AIResponseCompletedPayload{
					ConversationID: command.ConversationID, RequestID: command.RequestID, AssistantMessageID: assistant.ID,
				}
			} else {
				message := strings.TrimSpace(input.ErrorMessage)
				var walletPath *string
				var rechargePath *string
				if strings.TrimSpace(input.ErrorCode) == "ai.billing.insufficient_balance" {
					wallet := "/profile/wallet"
					recharge := "/payment/recharge"
					walletPath = &wallet
					rechargePath = &recharge
				}
				eventInput.Type = modulerealtime.TypeAIResponseFailedV1
				eventInput.Payload = modulerealtime.AIResponseFailedPayload{
					ConversationID: command.ConversationID, RequestID: command.RequestID, Msg: message, ErrorCode: strings.TrimSpace(input.ErrorCode), WalletPath: walletPath, RechargePath: rechargePath,
				}
			}
			var err error
			durableEvent, err = r.eventSink.AppendTx(ctx, tx, eventInput)
			if err != nil {
				return err
			}
		}
		resolved = true
		return nil
	})
	if err == nil && durableEvent != nil {
		r.eventSink.PublishBestEffort(ctx, durableEvent)
	}
	return resolved, err
}
