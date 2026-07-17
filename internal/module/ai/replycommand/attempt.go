package replycommand

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AttemptState string

const (
	AttemptPrepared       AttemptState = "prepared"
	AttemptDispatched     AttemptState = "dispatched"
	AttemptSucceeded      AttemptState = "succeeded"
	AttemptFailed         AttemptState = "failed"
	AttemptCanceled       AttemptState = "canceled"
	AttemptOutcomeUnknown AttemptState = "outcome_unknown"
)

type Attempt struct {
	ID                uint64       `gorm:"column:id;primaryKey"`
	CommandID         uint64       `gorm:"column:command_id"`
	AttemptNo         uint         `gorm:"column:attempt_no"`
	IdempotencyKey    string       `gorm:"column:idempotency_key"`
	State             AttemptState `gorm:"column:state"`
	ProviderRequestID string       `gorm:"column:provider_request_id"`
	ResponseSHA256    string       `gorm:"column:response_sha256"`
	ErrorCode         string       `gorm:"column:error_code"`
	DispatchedAt      *time.Time   `gorm:"column:dispatched_at"`
	FinishedAt        *time.Time   `gorm:"column:finished_at"`
	CreatedAt         time.Time    `gorm:"column:created_at"`
	UpdatedAt         time.Time    `gorm:"column:updated_at"`
}

func (Attempt) TableName() string { return "ai_provider_attempts" }

type PrepareAttemptInput struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Now       time.Time
}

type FinishAttemptInput struct {
	AttemptID         uint64
	CommandID         uint64
	Owner             string
	Token             uint64
	State             AttemptState
	ProviderRequestID string
	ResponseSHA256    string
	ErrorCode         string
	Now               time.Time
}

func (r *GormRepository) PrepareAttempt(ctx context.Context, input PrepareAttemptInput) (*Attempt, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	input.Owner = strings.TrimSpace(input.Owner)
	if input.CommandID == 0 || input.Owner == "" || input.Token == 0 {
		return nil, false, ErrCreateInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	var attempt *Attempt
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ? AND lease_owner = ? AND lease_token = ? AND state = ? AND cancel_requested_at IS NULL AND lease_expires_at > ?", input.CommandID, input.Owner, input.Token, StateRunning, input.Now).
			First(&command).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var maxAttempt uint
		if err := tx.Model(&Attempt{}).Where("command_id = ?", input.CommandID).Select("COALESCE(MAX(attempt_no), 0)").Scan(&maxAttempt).Error; err != nil {
			return err
		}
		row := &Attempt{
			CommandID:      input.CommandID,
			AttemptNo:      maxAttempt + 1,
			IdempotencyKey: providerAttemptKey(input.CommandID, maxAttempt+1),
			State:          AttemptPrepared,
			CreatedAt:      input.Now,
			UpdatedAt:      input.Now,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		attempt = row
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return attempt, attempt != nil, nil
}

func (r *GormRepository) MarkAttemptDispatched(ctx context.Context, attemptID uint64, commandID uint64, owner string, token uint64, now time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if attemptID == 0 || commandID == 0 || owner == "" || token == 0 {
		return false, ErrCreateInputInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}
	result := r.db.WithContext(ctx).Model(&Attempt{}).
		Where("id = ? AND command_id = ? AND state = ?", attemptID, commandID, AttemptPrepared).
		Where("EXISTS (SELECT 1 FROM ai_reply_commands c WHERE c.id = ? AND c.lease_owner = ? AND c.lease_token = ? AND c.state = ? AND c.cancel_requested_at IS NULL AND c.lease_expires_at > ?)", commandID, owner, token, StateRunning, now).
		Updates(map[string]any{"state": AttemptDispatched, "dispatched_at": now, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) FinishAttempt(ctx context.Context, input FinishAttemptInput) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	if input.AttemptID == 0 || input.CommandID == 0 || !terminalAttemptState(input.State) {
		return false, ErrCreateInputInvalid
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	updates := map[string]any{
		"state":               input.State,
		"provider_request_id": strings.TrimSpace(input.ProviderRequestID),
		"response_sha256":     strings.TrimSpace(input.ResponseSHA256),
		"error_code":          strings.TrimSpace(input.ErrorCode),
		"finished_at":         input.Now,
		"updated_at":          input.Now,
	}
	result := r.db.WithContext(ctx).Model(&Attempt{}).
		Where("id = ? AND command_id = ? AND state IN ?", input.AttemptID, input.CommandID, []AttemptState{AttemptPrepared, AttemptDispatched}).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func terminalAttemptState(state AttemptState) bool {
	switch state {
	case AttemptSucceeded, AttemptFailed, AttemptCanceled, AttemptOutcomeUnknown:
		return true
	default:
		return false
	}
}

func ambiguousAttemptState(state AttemptState) bool {
	switch state {
	case AttemptDispatched, AttemptSucceeded, AttemptOutcomeUnknown:
		return true
	default:
		return false
	}
}

func providerAttemptKey(commandID uint64, attemptNo uint) string {
	sum := sha256.Sum256([]byte("ai-provider:" + strconv.FormatUint(commandID, 10) + ":" + strconv.FormatUint(uint64(attemptNo), 10)))
	return hex.EncodeToString(sum[:])
}
