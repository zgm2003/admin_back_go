package replycommand

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPaidCommandFinalizationConflict = errors.New("paid reply command finalization conflict")

type PaidCommandFinalizationInput struct {
	CommandID    uint64
	UserID       int64
	RequestID    string
	State        State
	Content      string
	ErrorCode    string
	ErrorMessage string
	Now          time.Time
}

type PaidCommandFinalizationResult struct {
	AssistantMessageID  int64
	ConversationDeleted bool
}

func normalizePaidCommandFinalization(input *PaidCommandFinalizationInput) error {
	if input == nil {
		return ErrCreateInputInvalid
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Content = strings.TrimSpace(input.Content)
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	input.ErrorMessage = strings.TrimSpace(input.ErrorMessage)
	if input.CommandID == 0 || input.UserID <= 0 || input.RequestID == "" || input.Now.IsZero() {
		return ErrCreateInputInvalid
	}
	switch input.State {
	case StateSucceeded:
		if input.Content == "" || input.ErrorCode != "" || input.ErrorMessage != "" {
			return ErrCreateInputInvalid
		}
	case StateFailed, StateOutcomeUnknown:
		if input.Content != "" || input.ErrorCode == "" || input.ErrorMessage == "" {
			return ErrCreateInputInvalid
		}
	case StateCanceled:
		if input.Content != "" || input.ErrorCode != "" || input.ErrorMessage != "" {
			return ErrCreateInputInvalid
		}
	default:
		return ErrCreateInputInvalid
	}
	return nil
}

// FinalizePaidCommandInTransaction is the chat participant in the outer AI
// settlement transaction. The caller owns Run/Charge/wallet/Hold locking.
func (r *GormRepository) FinalizePaidCommandInTransaction(ctx context.Context, tx *gorm.DB, input PaidCommandFinalizationInput) (*PaidCommandFinalizationResult, error) {
	if r == nil || r.db == nil || tx == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := normalizePaidCommandFinalization(&input); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx = tx.WithContext(ctx)
	var command Command
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ? AND request_id = ?", input.CommandID, input.UserID, input.RequestID).
		First(&command).Error; err != nil {
		return nil, err
	}
	sourceStates, err := paidCommandFinalizationSourceStates(command.State, input.State)
	if err != nil {
		return nil, err
	}

	result := &PaidCommandFinalizationResult{}
	var existing replyMessage
	messageErr := tx.Where("reply_command_id = ?", command.ID).First(&existing).Error
	if messageErr != nil && !errors.Is(messageErr, gorm.ErrRecordNotFound) {
		return nil, messageErr
	}
	if input.State == StateSucceeded {
		if command.CancelRequestedAt != nil {
			return nil, ErrPaidCommandFinalizationConflict
		}
		if errors.Is(messageErr, gorm.ErrRecordNotFound) {
			commandID := command.ID
			deliveryState := DeliveryStateCompleted
			existing = replyMessage{
				ConversationID: command.ConversationID,
				ReplyCommandID: &commandID,
				Role:           enum.AIMessageRoleAssistant,
				ContentType:    "text",
				Content:        input.Content,
				DeliveryState:  &deliveryState,
				IsDel:          enum.CommonNo,
				CreatedAt:      input.Now,
				UpdatedAt:      input.Now,
			}
			if err := tx.Create(&existing).Error; err != nil {
				return nil, err
			}
		} else if existing.ConversationID != command.ConversationID || existing.Role != enum.AIMessageRoleAssistant || existing.Content != input.Content || existing.IsDel != enum.CommonNo {
			return nil, ErrPaidCommandFinalizationConflict
		}
		conversationUpdate := tx.Model(&replyConversation{}).
			Where("id = ? AND user_id = ? AND is_del = ?", command.ConversationID, command.UserID, enum.CommonNo).
			Updates(map[string]any{"last_message_at": input.Now, "updated_at": input.Now})
		if conversationUpdate.Error != nil {
			return nil, conversationUpdate.Error
		}
		if conversationUpdate.RowsAffected != 1 {
			return nil, ErrPaidCommandFinalizationConflict
		}
		result.AssistantMessageID = existing.ID
	} else if input.State == StateCanceled {
		if command.CancelRequestedAt == nil || command.StopDeliverySeq == nil || command.AssistantMessageID == nil ||
			errors.Is(messageErr, gorm.ErrRecordNotFound) || existing.ID != *command.AssistantMessageID ||
			existing.ConversationID != command.ConversationID || existing.Role != enum.AIMessageRoleAssistant ||
			existing.DeliveryState == nil || *existing.DeliveryState != DeliveryStateStopped {
			return nil, ErrPaidCommandFinalizationConflict
		}
		switch existing.IsDel {
		case enum.CommonNo:
		case enum.CommonYes:
			deleted, err := paidCommandConversationDeleted(tx, command)
			if err != nil || !deleted {
				return nil, ErrPaidCommandFinalizationConflict
			}
			result.ConversationDeleted = true
		default:
			return nil, ErrPaidCommandFinalizationConflict
		}
		result.AssistantMessageID = existing.ID
	} else if !errors.Is(messageErr, gorm.ErrRecordNotFound) || command.AssistantMessageID != nil {
		return nil, ErrPaidCommandFinalizationConflict
	} else {
		deleted, err := paidCommandConversationDeleted(tx, command)
		if err != nil {
			return nil, err
		}
		result.ConversationDeleted = deleted
	}

	updates := map[string]any{
		"state":            input.State,
		"finished_at":      input.Now,
		"lease_owner":      nil,
		"lease_expires_at": nil,
		"updated_at":       input.Now,
	}
	if input.State == StateSucceeded {
		updates["assistant_message_id"] = result.AssistantMessageID
		updates["last_error_code"] = ""
		updates["last_error_message"] = ""
		updates["outcome_unknown_at"] = nil
	} else if input.State == StateCanceled {
		updates["assistant_message_id"] = result.AssistantMessageID
		updates["last_error_code"] = ""
		updates["last_error_message"] = ""
		updates["outcome_unknown_at"] = nil
	} else {
		updates["last_error_code"] = input.ErrorCode
		updates["last_error_message"] = input.ErrorMessage
		if input.State == StateOutcomeUnknown {
			updates["outcome_unknown_at"] = input.Now
		} else {
			updates["outcome_unknown_at"] = nil
		}
	}
	update := tx.Model(&Command{}).
		Where("id = ? AND user_id = ? AND request_id = ? AND state IN ?", command.ID, command.UserID, command.RequestID, sourceStates).
		Updates(updates)
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected != 1 {
		return nil, ErrPaidCommandFinalizationConflict
	}
	return result, nil
}

func paidCommandConversationDeleted(tx *gorm.DB, command Command) (bool, error) {
	var conversation replyConversation
	if err := tx.Where("id = ? AND user_id = ?", command.ConversationID, command.UserID).First(&conversation).Error; err != nil {
		return false, err
	}
	switch conversation.IsDel {
	case enum.CommonNo:
		return false, nil
	case enum.CommonYes:
		return true, nil
	default:
		return false, ErrPaidCommandFinalizationConflict
	}
}

func paidCommandFinalizationSourceStates(current State, target State) ([]State, error) {
	states := []State{StatePending, StateClaimed, StateRunning, StateOutcomeUnknown}
	if current != StateFailed {
		return states, nil
	}
	if target != StateFailed {
		return nil, ErrPaidCommandFinalizationConflict
	}
	return append(states, StateFailed), nil
}
