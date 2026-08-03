package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemoryCommitDisposition string

const (
	MemoryCommitCreated  MemoryCommitDisposition = "created"
	MemoryCommitExisting MemoryCommitDisposition = "existing"
	MemoryCommitStale    MemoryCommitDisposition = "stale"
)

type MemoryBuildSnapshot struct {
	Payload               ContextMemoryBuildV1
	MemoryProviderModelID uint64
	MemoryMaxOutputTokens uint64
	Parent                *MemoryRecord
	Turns                 []ConversationTurn
	ParentSummary         string
	Prompt                string
}

type MemoryRepository interface {
	LoadMemoryBuild(context.Context, ContextMemoryBuildV1) (MemoryBuildSnapshot, error)
	CommitMemory(context.Context, ContextMemoryBuildV1, MemoryCandidate) (MemoryRecord, MemoryCommitDisposition, error)
}

type GormMemoryRepository struct {
	db    *gorm.DB
	turns ConversationTurnPager
}

func NewMemoryRepository(client *database.Client) *GormMemoryRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormMemoryRepository{db: client.Gorm, turns: NewConversationRepository(client)}
}

func NewMemoryRepositoryWithDB(db *gorm.DB, turns ConversationTurnPager) *GormMemoryRepository {
	if db == nil {
		return nil
	}
	return &GormMemoryRepository{db: db, turns: turns}
}

func MemoryIdentityKey(candidate MemoryCandidate) (string, error) {
	if candidate.ConversationID == 0 || candidate.ProfileID == 0 || candidate.ThroughMessageID == 0 || candidate.SourceSHA256 == ([sha256.Size]byte{}) {
		return "", ErrMemoryInvalid
	}
	return fmt.Sprintf("%d:%d:%d:%s", candidate.ConversationID, candidate.ProfileID, candidate.ThroughMessageID, hex.EncodeToString(candidate.SourceSHA256[:])), nil
}

func (repository *GormMemoryRepository) LoadMemoryBuild(ctx context.Context, payload ContextMemoryBuildV1) (MemoryBuildSnapshot, error) {
	if repository == nil || repository.db == nil || repository.turns == nil || payload.Validate() != nil {
		return MemoryBuildSnapshot{}, ErrMemoryInvalid
	}
	var terminal int64
	if err := repository.db.WithContext(ctx).Model(&MemoryRecord{}).
		Where("conversation_id = ? AND context_profile_id_snapshot = ? AND through_message_id = ? AND source_sha256 = ?", payload.ConversationID, payload.ProfileID, payload.ThroughMessageID, payload.SourceSHA256[:]).
		Count(&terminal).Error; err != nil {
		return MemoryBuildSnapshot{}, err
	}
	if terminal > 0 {
		return MemoryBuildSnapshot{}, ErrMemoryAlreadyTerminal
	}
	var profile ContextProfile
	if err := repository.db.WithContext(ctx).Where("id = ? AND status = ? AND memory_provider_model_id IS NOT NULL", payload.ProfileID, ProfileEnabled).Take(&profile).Error; err != nil {
		return MemoryBuildSnapshot{}, err
	}
	profileSHA, err := memoryProfileSHA256(profile)
	if err != nil || profileSHA != payload.ProfileSHA256 || profile.MemoryProviderModelID == nil {
		return MemoryBuildSnapshot{}, ErrMemorySnapshotStale
	}
	var parent *MemoryRecord
	if payload.PreviousMemoryID != nil {
		var row MemoryRecord
		if err := repository.db.WithContext(ctx).Where("id = ? AND state = ?", *payload.PreviousMemoryID, MemoryStateReady).Take(&row).Error; err != nil {
			return MemoryBuildSnapshot{}, err
		}
		parent = &row
	}
	var latest MemoryRecord
	latestErr := repository.db.WithContext(ctx).Where("conversation_id = ? AND context_profile_id_snapshot = ? AND state = ?", payload.ConversationID, payload.ProfileID, MemoryStateReady).
		Order("through_message_id DESC, id DESC").Take(&latest).Error
	if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
		return MemoryBuildSnapshot{}, latestErr
	}
	if errors.Is(latestErr, gorm.ErrRecordNotFound) {
		if payload.PreviousMemoryID != nil {
			return MemoryBuildSnapshot{}, ErrMemorySnapshotStale
		}
	} else if payload.PreviousMemoryID == nil || *payload.PreviousMemoryID != latest.ID {
		return MemoryBuildSnapshot{}, ErrMemorySnapshotStale
	}
	var conversation struct {
		UserID uint64 `gorm:"column:user_id"`
	}
	if err := repository.db.WithContext(ctx).Table("ai_conversations").Select("user_id").
		Where("id = ? AND is_del = ?", payload.ConversationID, enum.CommonNo).Take(&conversation).Error; err != nil {
		return MemoryBuildSnapshot{}, err
	}
	if err := validateMemoryRangeContinuity(ctx, repository.turns, payload.ConversationID, conversation.UserID, payload.FromMessageID, parent); err != nil {
		return MemoryBuildSnapshot{}, err
	}
	selected, err := memoryTurnsForRange(ctx, repository.turns, payload.ConversationID, conversation.UserID, payload.FromMessageID, payload.ThroughMessageID)
	if err != nil {
		return MemoryBuildSnapshot{}, err
	}
	parentSummary := ""
	if parent != nil {
		if parent.Summary == nil {
			return MemoryBuildSnapshot{}, ErrMemorySnapshotStale
		}
		parentSummary = *parent.Summary
	}
	prompt, err := BuildMemoryPrompt(parentSummary, selected)
	if err != nil {
		return MemoryBuildSnapshot{}, err
	}
	limits, err := loadMemoryModelLimits(ctx, repository.db, *profile.MemoryProviderModelID)
	if err != nil {
		return MemoryBuildSnapshot{}, err
	}
	return MemoryBuildSnapshot{Payload: payload, MemoryProviderModelID: *profile.MemoryProviderModelID, MemoryMaxOutputTokens: limits.MaxOutputTokens,
		Parent: parent, Turns: selected, ParentSummary: parentSummary, Prompt: prompt}, nil
}

func (repository *GormMemoryRepository) CommitMemory(ctx context.Context, payload ContextMemoryBuildV1, candidate MemoryCandidate) (MemoryRecord, MemoryCommitDisposition, error) {
	if repository == nil || repository.db == nil || payload.Validate() != nil || candidate.ValidateForInsert() != nil {
		return MemoryRecord{}, "", ErrMemoryInvalid
	}
	var stored MemoryRecord
	disposition := MemoryCommitStale
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation struct {
			ID     uint64
			UserID uint64 `gorm:"column:user_id"`
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("ai_conversations AS c").
			Select("c.id, c.user_id").Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ? AND a.context_profile_id = ?", enum.CommonNo, payload.ProfileID).
			Where("c.id = ? AND c.is_del = ?", payload.ConversationID, enum.CommonNo).Take(&conversation).Error; err != nil {
			return err
		}
		var profile ContextProfile
		if err := tx.Where("id = ? AND status = ?", payload.ProfileID, ProfileEnabled).Take(&profile).Error; err != nil {
			return err
		}
		profileSHA, err := memoryProfileSHA256(profile)
		if err != nil || profileSHA != payload.ProfileSHA256 {
			return ErrMemorySnapshotStale
		}
		var latest MemoryRecord
		latestErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("conversation_id = ? AND context_profile_id_snapshot = ? AND state = ?", payload.ConversationID, payload.ProfileID, MemoryStateReady).
			Order("through_message_id DESC, id DESC").Take(&latest).Error
		if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
			return latestErr
		}
		if errors.Is(latestErr, gorm.ErrRecordNotFound) {
			if payload.PreviousMemoryID != nil || candidate.ParentMemoryID != nil {
				return ErrMemorySnapshotStale
			}
		} else {
			if payload.PreviousMemoryID == nil || *payload.PreviousMemoryID != latest.ID || candidate.ParentMemoryID == nil || *candidate.ParentMemoryID != latest.ID {
				return ErrMemorySnapshotStale
			}
			if err := ValidateMemoryParent(candidate, latest); err != nil {
				return err
			}
		}
		if candidate.ConversationID != payload.ConversationID || candidate.ProfileID != payload.ProfileID || candidate.FromMessageID != payload.FromMessageID ||
			candidate.ThroughMessageID != payload.ThroughMessageID || candidate.SourceSHA256 != payload.SourceSHA256 || candidate.PolicyVersion != payload.PolicyVersion {
			return ErrMemorySnapshotStale
		}
		turnRepository := NewConversationRepositoryWithDB(tx)
		var continuityParent *MemoryRecord
		if latestErr == nil {
			continuityParent = &latest
		}
		if err := validateMemoryRangeContinuity(ctx, turnRepository, payload.ConversationID, conversation.UserID, payload.FromMessageID, continuityParent); err != nil {
			return ErrMemorySnapshotStale
		}
		turns, err := memoryTurnsForRange(ctx, turnRepository, payload.ConversationID, conversation.UserID, payload.FromMessageID, payload.ThroughMessageID)
		if err != nil {
			return ErrMemorySnapshotStale
		}
		sourceInput := MemorySourceInput{ProfileID: payload.ProfileID, ProfileSHA256: payload.ProfileSHA256,
			ConversationID: payload.ConversationID, ParentMemoryID: payload.PreviousMemoryID, Turns: turns}
		if latestErr == nil {
			sourceInput.ParentSummarySHA256 = parentSummaryHash(&latest)
		}
		currentSource, err := MemorySourceSHA256(sourceInput)
		if err != nil || currentSource != payload.SourceSHA256 {
			return ErrMemorySnapshotStale
		}
		row := memoryRowFromCandidate(candidate)
		if err := tx.Create(&row).Error; err != nil {
			var mysqlErr *mysqldriver.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
				return err
			}
			if err := tx.Where("conversation_id = ? AND context_profile_id_snapshot = ? AND through_message_id = ? AND source_sha256 = ?", candidate.ConversationID, candidate.ProfileID, candidate.ThroughMessageID, candidate.SourceSHA256[:]).Take(&stored).Error; err != nil {
				return err
			}
			disposition = MemoryCommitExisting
			return nil
		}
		stored = row
		disposition = MemoryCommitCreated
		return nil
	})
	if errors.Is(err, ErrMemorySnapshotStale) {
		return MemoryRecord{}, MemoryCommitStale, nil
	}
	return stored, disposition, err
}
