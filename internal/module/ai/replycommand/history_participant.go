package replycommand

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

var ErrHistoryTransactionRequired = errors.New("reply history participant requires an active caller transaction")

type HistoryRequest struct {
	UserID    int64
	RequestID string
	Identity  requestidentity.Input
	// ResolveIdentity is evaluated only after the canonical request key exists.
	ResolveIdentity func(context.Context, *gorm.DB) (requestidentity.Input, error)
}

type HistoryCreateInput struct {
	HistoryRequest
	ConversationID      int64
	AgentID             int64
	ProviderID          int64
	ModelID             string
	ModelDisplayName    string
	Content             string
	MetaJSON            *string
	InputSnapshot       string
	PricingSnapshotJSON string
	EffectiveMaxTokens  int64
	AcceptedAt          time.Time
}

type HistoryTransactionParticipant interface {
	ReplayInTransaction(context.Context, *gorm.DB, HistoryRequest) (*CreateReplyResult, error)
	CreateInTransaction(context.Context, *gorm.DB, HistoryCreateInput) (CreateReplyResult, error)
}

type HistoryParticipant struct{ repository *GormRepository }

func NewHistoryParticipant(repository *GormRepository) *HistoryParticipant {
	return &HistoryParticipant{repository: repository}
}

func (p *HistoryParticipant) ReplayInTransaction(ctx context.Context, tx *gorm.DB, request HistoryRequest) (*CreateReplyResult, error) {
	if err := requireHistoryTransaction(tx); err != nil {
		return nil, err
	}
	if err := validateHistoryRequestKey(request); err != nil {
		return nil, err
	}
	var command Command
	err := tx.WithContext(nonNilContext(ctx)).
		Where("user_id = ? AND request_id = ?", request.UserID, strings.TrimSpace(request.RequestID)).
		First(&command).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var canonical struct {
			ID                    int64  `gorm:"column:id"`
			RequestFingerprint    []byte `gorm:"column:request_fingerprint"`
			RequestIdentityStatus string `gorm:"column:request_identity_status"`
		}
		err = tx.WithContext(nonNilContext(ctx)).Table("ai_runs").
			Select("id, request_fingerprint, request_identity_status").
			Where("user_id = ? AND request_id = ?", request.UserID, strings.TrimSpace(request.RequestID)).
			Take(&canonical).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		fingerprint, err := resolveHistoryRequestFingerprint(ctx, tx, request)
		if err != nil {
			return nil, err
		}
		if len(canonical.RequestFingerprint) != len(fingerprint) {
			return nil, requestidentity.ErrRequestIdentityNotReplayable
		}
		var stored [32]byte
		copy(stored[:], canonical.RequestFingerprint)
		if err := requestidentity.CompareForReplay(requestidentity.IdentityStatus(canonical.RequestIdentityStatus), stored, fingerprint); err != nil {
			return nil, err
		}
		return nil, requestidentity.ErrRequestIdentityConflict
	}
	if err != nil {
		return nil, err
	}
	fingerprint, err := resolveHistoryRequestFingerprint(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	if err := compareCommandFingerprint(command, fingerprint); err != nil {
		return nil, err
	}
	accepted, err := loadAcceptedRunCharge(tx.WithContext(nonNilContext(ctx)), request.UserID, strings.TrimSpace(request.RequestID), false)
	if err != nil {
		return nil, err
	}
	return &CreateReplyResult{
		UserMessageID: command.UserMessageID, CommandID: command.ID, RunID: accepted.RunID, ChargeID: accepted.ChargeID,
		RequestID: command.RequestID, State: command.State,
	}, nil
}

func (p *HistoryParticipant) CreateInTransaction(ctx context.Context, tx *gorm.DB, input HistoryCreateInput) (CreateReplyResult, error) {
	if err := requireHistoryTransaction(tx); err != nil {
		return CreateReplyResult{}, err
	}
	if p == nil || p.repository == nil || p.repository.db == nil {
		return CreateReplyResult{}, ErrRepositoryNotConfigured
	}
	fingerprint, err := validateHistoryCreateInput(input)
	if err != nil {
		return CreateReplyResult{}, err
	}
	now := input.AcceptedAt
	if now.IsZero() {
		now = time.Now()
		if p.repository.now != nil {
			now = p.repository.now()
		}
	}
	db := tx.WithContext(nonNilContext(ctx))
	message := replyMessage{
		ConversationID: input.ConversationID, Role: enum.AIMessageRoleUser, ContentType: "text",
		Content: input.Content, MetaJSON: cloneStringPointer(input.MetaJSON), IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		return CreateReplyResult{}, err
	}

	requestID := strings.TrimSpace(input.RequestID)
	key := idempotencyKey(input.UserID, requestID)
	if p.repository.idempotencyKey != nil {
		key = p.repository.idempotencyKey(input.UserID, requestID)
	}
	conversationID, userMessageID, idempotency := input.ConversationID, message.ID, key
	run := airun.Run{
		Platform: enum.PlatformAdmin, ConversationID: &conversationID, RequestID: requestID,
		RequestFingerprint: append([]byte(nil), fingerprint[:]...), RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
		RequestIdentityMarker: "", IdempotencyKey: &idempotency, UserMessageID: &userMessageID, UserID: input.UserID,
		AgentID: input.AgentID, ProviderID: input.ProviderID, ModelID: strings.TrimSpace(input.ModelID),
		ModelDisplayName: strings.TrimSpace(input.ModelDisplayName), InputSnapshot: strings.TrimSpace(input.InputSnapshot),
		PricingSnapshotJSON: strings.TrimSpace(input.PricingSnapshotJSON), Status: enum.AIRunStatusRunning,
		BillingStatus: string(billing.BillingStatusPending), BillingReason: string(billing.BillingReasonPending),
		StartedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&run).Error; err != nil {
		return CreateReplyResult{}, err
	}
	command := Command{
		RequestID: requestID, RequestFingerprint: append([]byte(nil), fingerprint[:]...),
		RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RequestIdentityMarker: "",
		IdempotencyKey: key, Platform: enum.PlatformAdmin, UserID: input.UserID, ConversationID: input.ConversationID,
		RunID: run.ID, UserMessageID: message.ID, State: StatePending, MaxAttempts: defaultMaxAttempts, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&command).Error; err != nil {
		return CreateReplyResult{}, err
	}
	if err := db.Create(&airun.RunEvent{RunID: run.ID, Seq: 1, EventType: enum.AIRunEventStart, Message: enum.AIRunEventLabels[enum.AIRunEventStart], CreatedAt: now}).Error; err != nil {
		return CreateReplyResult{}, err
	}
	pricingSnapshot, _ := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
	charge := billing.UsageCharge{
		RunID: run.ID, UserID: input.UserID, Currency: "CNY", PricingVersion: pricingSnapshot.Version,
		MultiplierPPM: pricingSnapshot.MultiplierPPM, Status: billing.ChargeStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&charge).Error; err != nil {
		return CreateReplyResult{}, err
	}
	return CreateReplyResult{
		UserMessageID: message.ID, CommandID: command.ID, RunID: run.ID, ChargeID: charge.ID, RequestID: requestID, State: command.State,
	}, nil
}

func validateHistoryRequestKey(request HistoryRequest) error {
	requestID := strings.TrimSpace(request.RequestID)
	if request.UserID <= 0 || requestID == "" || utf8.RuneCountInString(requestID) > 128 {
		return ErrCreateInputInvalid
	}
	return nil
}

func resolveHistoryRequestFingerprint(ctx context.Context, tx *gorm.DB, request HistoryRequest) ([32]byte, error) {
	identity := request.Identity
	if request.ResolveIdentity != nil {
		resolved, err := request.ResolveIdentity(nonNilContext(ctx), tx)
		if err != nil {
			return [32]byte{}, err
		}
		identity = resolved
	}
	if identity.UserID != request.UserID {
		return [32]byte{}, ErrCreateInputInvalid
	}
	fingerprint, err := requestidentity.BuildFingerprint(identity)
	if err != nil {
		return [32]byte{}, fmt.Errorf("%w: invalid typed history request identity: %v", ErrCreateInputInvalid, err)
	}
	return fingerprint, nil
}

func validateHistoryCreateInput(input HistoryCreateInput) ([32]byte, error) {
	if input.ResolveIdentity != nil {
		return [32]byte{}, ErrCreateInputInvalid
	}
	if err := validateHistoryRequestKey(input.HistoryRequest); err != nil {
		return [32]byte{}, err
	}
	fingerprint, err := resolveHistoryRequestFingerprint(context.Background(), nil, input.HistoryRequest)
	if err != nil {
		return [32]byte{}, err
	}
	modelID := strings.TrimSpace(input.ModelID)
	pricingSnapshot, pricingErr := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
	if input.ConversationID <= 0 || input.AgentID <= 0 || input.ProviderID <= 0 || modelID == "" ||
		strings.TrimSpace(input.InputSnapshot) == "" || input.EffectiveMaxTokens <= 0 ||
		input.Identity.ConversationID != input.ConversationID || input.Identity.AgentID != input.AgentID || input.Identity.ModelID != modelID ||
		pricingErr != nil || pricingSnapshot.RequestedModelID != modelID || int64(pricingSnapshot.EffectiveMaxOutputTokens) != input.EffectiveMaxTokens {
		return [32]byte{}, ErrCreateInputInvalid
	}
	return fingerprint, nil
}

func requireHistoryTransaction(tx *gorm.DB) error {
	if err := requireAttemptTransaction(tx); err != nil {
		return ErrHistoryTransactionRequired
	}
	return nil
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

var _ HistoryTransactionParticipant = (*HistoryParticipant)(nil)
