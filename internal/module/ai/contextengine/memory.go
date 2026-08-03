package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MemoryPolicyVersionV1  = ContextPolicyV1
	MemoryStateReady       = "ready"
	MemoryStateFailed      = "failed"
	MemoryStateInvalidated = "invalidated"
)

var (
	ErrMemoryInvalid         = errors.New("invalid conversation memory")
	ErrMemoryIDMustBeZero    = errors.New("new conversation memory id must be zero")
	ErrMemorySelfParent      = errors.New("conversation memory cannot parent itself")
	ErrMemoryParentScope     = errors.New("conversation memory parent has a different scope")
	ErrMemoryParentGap       = errors.New("conversation memory parent interval is not contiguous")
	ErrMemorySnapshotStale   = errors.New("conversation memory snapshot is stale")
	ErrMemoryAlreadyTerminal = errors.New("conversation memory identity is already terminal")
)

type MemoryRecord struct {
	ID                uint64    `gorm:"column:id;primaryKey"`
	ConversationID    uint64    `gorm:"column:conversation_id"`
	ProfileID         uint64    `gorm:"column:context_profile_id_snapshot"`
	ProfileSHA256     []byte    `gorm:"column:context_profile_sha256"`
	ParentMemoryID    *uint64   `gorm:"column:previous_memory_id"`
	FromMessageID     uint64    `gorm:"column:from_message_id"`
	ThroughMessageID  uint64    `gorm:"column:through_message_id"`
	SourceSHA256      []byte    `gorm:"column:source_sha256"`
	SummarySHA256     []byte    `gorm:"column:summary_sha256"`
	PolicyVersion     string    `gorm:"column:policy_version"`
	Summary           *string   `gorm:"column:summary"`
	PromptTokens      *uint64   `gorm:"column:prompt_tokens"`
	CompletionTokens  *uint64   `gorm:"column:completion_tokens"`
	ProviderRequestID *string   `gorm:"column:provider_request_id"`
	State             string    `gorm:"column:state"`
	ErrorCode         *string   `gorm:"column:error_code"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (MemoryRecord) TableName() string { return "ai_conversation_memories" }

type MemoryCandidate struct {
	ID                uint64
	ConversationID    uint64
	ProfileID         uint64
	ProfileSHA256     [sha256.Size]byte
	ParentMemoryID    *uint64
	FromMessageID     uint64
	ThroughMessageID  uint64
	SourceSHA256      [sha256.Size]byte
	SummarySHA256     [sha256.Size]byte
	PolicyVersion     string
	Summary           string
	PromptTokens      *uint64
	CompletionTokens  *uint64
	ProviderRequestID string
	State             string
	ErrorCode         string
}

func (candidate MemoryCandidate) ValidateForInsert() error {
	if candidate.ID != 0 {
		return ErrMemoryIDMustBeZero
	}
	if candidate.ParentMemoryID != nil && *candidate.ParentMemoryID == candidate.ID {
		return ErrMemorySelfParent
	}
	if candidate.ConversationID == 0 || candidate.ProfileID == 0 || candidate.ProfileSHA256 == ([sha256.Size]byte{}) ||
		candidate.FromMessageID == 0 || candidate.ThroughMessageID < candidate.FromMessageID ||
		candidate.SourceSHA256 == ([sha256.Size]byte{}) || candidate.PolicyVersion != MemoryPolicyVersionV1 {
		return ErrMemoryInvalid
	}
	if (candidate.PromptTokens == nil) != (candidate.CompletionTokens == nil) {
		return ErrMemoryInvalid
	}
	switch candidate.State {
	case MemoryStateReady:
		if strings.TrimSpace(candidate.Summary) == "" || candidate.SummarySHA256 == ([sha256.Size]byte{}) || strings.TrimSpace(candidate.ErrorCode) != "" {
			return ErrMemoryInvalid
		}
	case MemoryStateFailed:
		if candidate.Summary != "" || candidate.SummarySHA256 != ([sha256.Size]byte{}) || strings.TrimSpace(candidate.ErrorCode) == "" {
			return ErrMemoryInvalid
		}
	default:
		return ErrMemoryInvalid
	}
	return nil
}

func ValidateMemoryParent(candidate MemoryCandidate, parent MemoryRecord) error {
	if candidate.ParentMemoryID == nil || *candidate.ParentMemoryID == 0 || *candidate.ParentMemoryID != parent.ID {
		return ErrMemoryParentScope
	}
	if parent.ID == candidate.ID {
		return ErrMemorySelfParent
	}
	if parent.ConversationID != candidate.ConversationID || parent.ProfileID != candidate.ProfileID || parent.State != MemoryStateReady {
		return ErrMemoryParentScope
	}
	if candidate.FromMessageID <= parent.ThroughMessageID {
		return ErrMemoryParentGap
	}
	return nil
}

type MemoryBuildWindow struct {
	HighWatermarkTokens uint64
	TargetTokens        uint64
}

func MemoryWindow(uncoveredTokens, knownInputBudget uint64) (MemoryBuildWindow, bool) {
	if knownInputBudget == 0 {
		return MemoryBuildWindow{}, false
	}
	window := MemoryBuildWindow{HighWatermarkTokens: knownInputBudget / 4, TargetTokens: knownInputBudget / 8}
	if uncoveredTokens <= window.HighWatermarkTokens {
		return MemoryBuildWindow{}, false
	}
	return window, true
}

type MemorySourceInput struct {
	ProfileID           uint64
	ProfileSHA256       [sha256.Size]byte
	ConversationID      uint64
	ParentMemoryID      *uint64
	ParentSummarySHA256 [sha256.Size]byte
	Turns               []ConversationTurn
}

type canonicalMemorySource struct {
	Schema              string   `json:"schema"`
	ProfileID           uint64   `json:"profile_id"`
	ProfileSHA256       string   `json:"profile_sha256"`
	ConversationID      uint64   `json:"conversation_id"`
	ParentMemoryID      *uint64  `json:"previous_memory_id"`
	ParentSummarySHA256 string   `json:"previous_summary_sha256,omitempty"`
	TurnSHA256          []string `json:"turn_sha256"`
}

func MemorySourceSHA256(input MemorySourceInput) ([sha256.Size]byte, error) {
	if input.ProfileID == 0 || input.ConversationID == 0 || input.ProfileSHA256 == ([sha256.Size]byte{}) || len(input.Turns) == 0 ||
		(input.ParentMemoryID == nil) != (input.ParentSummarySHA256 == ([sha256.Size]byte{})) {
		return [sha256.Size]byte{}, ErrMemoryInvalid
	}
	canonical := canonicalMemorySource{Schema: MemoryPolicyVersionV1, ProfileID: input.ProfileID,
		ProfileSHA256: hex.EncodeToString(input.ProfileSHA256[:]), ConversationID: input.ConversationID,
		ParentMemoryID: input.ParentMemoryID, TurnSHA256: make([]string, len(input.Turns))}
	if input.ParentMemoryID != nil {
		if *input.ParentMemoryID == 0 {
			return [sha256.Size]byte{}, ErrMemoryInvalid
		}
		canonical.ParentSummarySHA256 = hex.EncodeToString(input.ParentSummarySHA256[:])
	}
	var previous uint64
	for index, turn := range input.Turns {
		if turn.ConversationID != input.ConversationID || turn.UserMessage.ID <= previous {
			return [sha256.Size]byte{}, ErrMemoryInvalid
		}
		hash, err := ConversationTurnSourceSHA256(turn)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		canonical.TurnSHA256[index] = hex.EncodeToString(hash[:])
		previous = turn.UserMessage.ID
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("marshal memory source: %w", err)
	}
	return sha256.Sum256(raw), nil
}

type MemorySummaryRequest struct {
	ProviderModelID uint64
	Prompt          string
	MaxOutputTokens uint64
}

type MemorySummaryResult struct {
	Summary           string
	PromptTokens      uint64
	CompletionTokens  uint64
	ProviderRequestID string
}

type MemorySummarizer interface {
	Summarize(context.Context, MemorySummaryRequest) (MemorySummaryResult, error)
}

type MemoryServiceDependencies struct {
	Repository MemoryRepository
	Summarizer MemorySummarizer
}

type MemoryService struct {
	repository MemoryRepository
	summarizer MemorySummarizer
}

func NewMemoryService(deps MemoryServiceDependencies) *MemoryService {
	return &MemoryService{repository: deps.Repository, summarizer: deps.Summarizer}
}

func (service *MemoryService) BuildMemory(ctx context.Context, payload ContextMemoryBuildV1) error {
	if service == nil || service.repository == nil || service.summarizer == nil {
		return ErrMemoryInvalid
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	snapshot, err := service.repository.LoadMemoryBuild(ctx, payload)
	if errors.Is(err, ErrMemoryAlreadyTerminal) || errors.Is(err, ErrMemorySnapshotStale) {
		return nil
	}
	if err != nil {
		return err
	}
	source, err := MemorySourceSHA256(MemorySourceInput{ProfileID: payload.ProfileID, ProfileSHA256: payload.ProfileSHA256,
		ConversationID: payload.ConversationID, ParentMemoryID: payload.PreviousMemoryID,
		ParentSummarySHA256: parentSummaryHash(snapshot.Parent), Turns: snapshot.Turns})
	if err != nil || source != payload.SourceSHA256 {
		return nil
	}
	if snapshot.MemoryMaxOutputTokens == 0 {
		return ErrMemorySnapshotStale
	}
	result, err := service.summarizer.Summarize(ctx, MemorySummaryRequest{ProviderModelID: snapshot.MemoryProviderModelID, Prompt: snapshot.Prompt, MaxOutputTokens: snapshot.MemoryMaxOutputTokens})
	if err != nil {
		if code, permanent := permanentMemoryFailure(err); permanent {
			candidate := failedMemoryCandidate(payload, code)
			_, disposition, commitErr := service.repository.CommitMemory(ctx, payload, candidate)
			if commitErr != nil {
				return commitErr
			}
			if disposition == MemoryCommitStale {
				return nil
			}
			return &MemoryPermanentError{Code: code, Cause: err}
		}
		return err
	}
	if strings.TrimSpace(result.Summary) == "" {
		return errors.New("memory provider returned an empty summary")
	}
	summaryHash := sha256.Sum256([]byte(result.Summary))
	promptTokens, completionTokens := result.PromptTokens, result.CompletionTokens
	candidate := MemoryCandidate{ConversationID: payload.ConversationID, ProfileID: payload.ProfileID, ProfileSHA256: payload.ProfileSHA256,
		ParentMemoryID: payload.PreviousMemoryID, FromMessageID: payload.FromMessageID, ThroughMessageID: payload.ThroughMessageID,
		SourceSHA256: payload.SourceSHA256, SummarySHA256: summaryHash, PolicyVersion: payload.PolicyVersion, Summary: result.Summary,
		PromptTokens: &promptTokens, CompletionTokens: &completionTokens, ProviderRequestID: result.ProviderRequestID, State: MemoryStateReady}
	_, disposition, err := service.repository.CommitMemory(ctx, payload, candidate)
	if err != nil {
		return err
	}
	if disposition == MemoryCommitStale {
		return nil
	}
	return nil
}

func (service *MemoryService) FinalizeMemory(ctx context.Context, payload ContextMemoryBuildV1, _ int) error {
	if service == nil || service.repository == nil {
		return ErrMemoryInvalid
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	candidate := failedMemoryCandidate(payload, "ai.context.memory_retry_exhausted")
	_, disposition, err := service.repository.CommitMemory(ctx, payload, candidate)
	if err != nil {
		return err
	}
	if disposition == MemoryCommitStale {
		return ErrMemorySnapshotStale
	}
	return nil
}

func failedMemoryCandidate(payload ContextMemoryBuildV1, code string) MemoryCandidate {
	return MemoryCandidate{ConversationID: payload.ConversationID, ProfileID: payload.ProfileID, ProfileSHA256: payload.ProfileSHA256,
		ParentMemoryID: payload.PreviousMemoryID, FromMessageID: payload.FromMessageID, ThroughMessageID: payload.ThroughMessageID,
		SourceSHA256: payload.SourceSHA256, PolicyVersion: payload.PolicyVersion, State: MemoryStateFailed, ErrorCode: code}
}
