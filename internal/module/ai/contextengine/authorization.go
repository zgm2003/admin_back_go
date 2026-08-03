package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AuthoritySource struct {
	SourceType   string
	SourceRef    string
	SourceSHA256 [sha256.Size]byte
}

type lockedReplyCommand struct {
	ID                uint64     `gorm:"column:id"`
	State             string     `gorm:"column:state"`
	RequestID         string     `gorm:"column:request_id"`
	UserID            int64      `gorm:"column:user_id"`
	ConversationID    int64      `gorm:"column:conversation_id"`
	UserMessageID     int64      `gorm:"column:user_message_id"`
	LeaseOwner        *string    `gorm:"column:lease_owner"`
	LeaseToken        uint64     `gorm:"column:lease_token"`
	LeaseExpiresAt    *time.Time `gorm:"column:lease_expires_at"`
	CancelRequestedAt *time.Time `gorm:"column:cancel_requested_at"`
}

func (lockedReplyCommand) TableName() string { return "ai_reply_commands" }

type PlanAuthoritySnapshot struct {
	InputFingerprintSHA256 [sha256.Size]byte
	Fingerprint            InputFingerprintHashInput
	Sources                []AuthoritySource
}

func (snapshot PlanAuthoritySnapshot) Validate() error {
	if isZeroSHA256(snapshot.InputFingerprintSHA256) {
		return ErrInvalidPlanCommitToken
	}
	fingerprintHash, err := HashInputFingerprint(snapshot.Fingerprint)
	if err != nil || fingerprintHash != snapshot.InputFingerprintSHA256 {
		return ErrInvalidPlanCommitToken
	}
	seen := make(map[string]struct{}, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		if !sourceTypePattern.MatchString(source.SourceType) || strings.TrimSpace(source.SourceRef) == "" ||
			isZeroSHA256(source.SourceSHA256) {
			return ErrInvalidPlanCommitToken
		}
		key := source.SourceType + "\x00" + source.SourceRef
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalidPlanCommitToken
		}
		seen[key] = struct{}{}
	}
	return nil
}

func HashPlanAuthoritySnapshot(snapshot PlanAuthoritySnapshot) ([sha256.Size]byte, error) {
	if err := snapshot.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	type sourceCanonical struct {
		SourceType string `json:"source_type"`
		SourceRef  string `json:"source_ref"`
		SHA256     string `json:"source_sha256"`
	}
	sources := make([]sourceCanonical, len(snapshot.Sources))
	for i, source := range snapshot.Sources {
		sources[i] = sourceCanonical{SourceType: source.SourceType, SourceRef: source.SourceRef, SHA256: hex.EncodeToString(source.SourceSHA256[:])}
	}
	return canonicalSHA256(struct {
		Schema                 string            `json:"schema"`
		InputFingerprintSHA256 string            `json:"input_fingerprint_sha256"`
		Sources                []sourceCanonical `json:"sources"`
	}{Schema: "plan_authority_snapshot_v1", InputFingerprintSHA256: hex.EncodeToString(snapshot.InputFingerprintSHA256[:]), Sources: sources})
}

type PlanCommitGuardFactory interface {
	GuardFor(PlanAuthoritySnapshot) (PlanCommitTransactionGuard, [sha256.Size]byte, error)
}

type AuthoritySnapshotLoader interface {
	ReloadPlanAuthorityInTransaction(context.Context, *gorm.DB, PlanAuthoritySnapshot) (PlanAuthoritySnapshot, error)
}

type AuthorizationGuardFactory struct {
	loader AuthoritySnapshotLoader
	now    func() time.Time
}

func NewAuthorizationGuardFactory(loader AuthoritySnapshotLoader, now func() time.Time) *AuthorizationGuardFactory {
	if loader == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &AuthorizationGuardFactory{loader: loader, now: now}
}

func (factory *AuthorizationGuardFactory) GuardFor(snapshot PlanAuthoritySnapshot) (PlanCommitTransactionGuard, [sha256.Size]byte, error) {
	if factory == nil || factory.loader == nil || factory.now == nil {
		return nil, [sha256.Size]byte{}, ErrNilPlanCommitGuard
	}
	expected := clonePlanAuthoritySnapshot(snapshot)
	hash, err := HashPlanAuthoritySnapshot(expected)
	if err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	return &authorizationGuard{expected: expected, expectedHash: hash, loader: factory.loader, now: factory.now}, hash, nil
}

type authorizationGuard struct {
	expected     PlanAuthoritySnapshot
	expectedHash [sha256.Size]byte
	loader       AuthoritySnapshotLoader
	now          func() time.Time
}

func (guard *authorizationGuard) GuardPlanCommitInTransaction(ctx context.Context, tx *gorm.DB, token PlanCommitToken) (PlanCommitGuardResult, error) {
	if guard == nil || tx == nil || guard.loader == nil || guard.now == nil || token.AuthoritySnapshotSHA256 != guard.expectedHash {
		return PlanCommitGuardResult{}, ErrInvalidPlanCommitToken
	}
	var run airun.Run
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.RunID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PlanCommitGuardResult{}, ErrPlanCommitAborted
		}
		return PlanCommitGuardResult{}, err
	}
	var command lockedReplyCommand
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.ReplyCommandID).Take(&command).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PlanCommitGuardResult{}, ErrPlanCommitAborted
		}
		return PlanCommitGuardResult{}, err
	}
	if err := validateLockedPlanAuthority(run, command, token, guard.now().UTC()); err != nil {
		return PlanCommitGuardResult{}, err
	}
	current, err := guard.loader.ReloadPlanAuthorityInTransaction(ctx, tx, guard.expected)
	if err != nil {
		return PlanCommitGuardResult{}, err
	}
	currentHash, err := HashPlanAuthoritySnapshot(current)
	if err != nil {
		return PlanCommitGuardResult{}, err
	}
	if currentHash != guard.expectedHash {
		planError, err := NewPlanError("authorization", ErrCodeSnapshotConflict)
		if err != nil {
			return PlanCommitGuardResult{}, err
		}
		return PlanCommitGuardResult{SnapshotConflict: &planError}, nil
	}
	return PlanCommitGuardResult{}, nil
}

func validateLockedPlanAuthority(run airun.Run, command lockedReplyCommand, token PlanCommitToken, now time.Time) error {
	if run.ID != int64(token.RunID) || command.ID != token.ReplyCommandID || run.Status != enum.AIRunStatusRunning ||
		command.State != "running" || command.CancelRequestedAt != nil || command.LeaseOwner == nil ||
		*command.LeaseOwner != token.LeaseOwner || command.LeaseToken != token.LeaseToken || command.LeaseExpiresAt == nil ||
		!command.LeaseExpiresAt.After(now) {
		return ErrPlanCommitAborted
	}
	if run.RequestID == "" || run.RequestID != command.RequestID || run.UserID != command.UserID ||
		run.ConversationID == nil || *run.ConversationID != command.ConversationID ||
		run.UserMessageID == nil || *run.UserMessageID != command.UserMessageID {
		return fmt.Errorf("%w: run and reply command identity disagree", ErrInvalidPlanCommitToken)
	}
	return nil
}

func clonePlanAuthoritySnapshot(snapshot PlanAuthoritySnapshot) PlanAuthoritySnapshot {
	cloned := snapshot
	cloned.Fingerprint = cloneInputFingerprint(snapshot.Fingerprint)
	cloned.Sources = append([]AuthoritySource(nil), snapshot.Sources...)
	return cloned
}

func cloneInputFingerprint(input InputFingerprintHashInput) InputFingerprintHashInput {
	cloned := input
	cloned.Profile = cloneProfileSnapshot(input.Profile)
	cloned.Messages = make([]FingerprintMessage, len(input.Messages))
	for index, message := range input.Messages {
		cloned.Messages[index] = message
		cloned.Messages[index].Attachments = append([]FingerprintAttachment(nil), message.Attachments...)
	}
	cloned.Bindings = append([]FingerprintBinding(nil), input.Bindings...)
	cloned.Tools = append([]FingerprintTool(nil), input.Tools...)
	cloned.Generation.Temperature = clonePointer(input.Generation.Temperature)
	return cloned
}
