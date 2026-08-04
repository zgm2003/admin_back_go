package contextengine

import (
	"context"
	"errors"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
)

func (repository *GormMemoryRepository) ListMemoryBuildPayloads(ctx context.Context, afterConversationID uint64, limit int) ([]ContextMemoryBuildV1, uint64, error) {
	if repository == nil || repository.db == nil || repository.turns == nil || limit <= 0 {
		return nil, 0, ErrMemoryInvalid
	}
	var conversations []struct {
		ID     uint64 `gorm:"column:id"`
		UserID uint64 `gorm:"column:user_id"`
	}
	err := repository.db.WithContext(ctx).Table("ai_conversations AS c").Select("c.id, c.user_id").
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ? AND a.context_profile_id IS NOT NULL", enum.CommonNo).
		Joins("JOIN ai_context_profiles AS p ON p.id = a.context_profile_id AND p.status = ? AND p.memory_provider_model_id IS NOT NULL", ProfileEnabled).
		Where("c.id > ? AND c.is_del = ?", afterConversationID, enum.CommonNo).Order("c.id ASC").Limit(limit).Scan(&conversations).Error
	if err != nil {
		return nil, afterConversationID, err
	}
	next := uint64(0)
	if len(conversations) == limit {
		next = conversations[len(conversations)-1].ID
	}
	payloads := make([]ContextMemoryBuildV1, 0, len(conversations))
	for _, conversation := range conversations {
		payload, ok, err := repository.memoryBuildPayload(ctx, conversation.ID, conversation.UserID)
		if err != nil {
			return payloads, next, err
		}
		if ok {
			payloads = append(payloads, payload)
		}
	}
	return payloads, next, nil
}

func (repository *GormMemoryRepository) memoryBuildPayload(ctx context.Context, conversationID, userID uint64) (ContextMemoryBuildV1, bool, error) {
	profile, budget, err := repository.memoryBuildAuthority(ctx, conversationID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ContextMemoryBuildV1{}, false, nil
	}
	if err != nil || budget.KnownInputBudget == 0 {
		return ContextMemoryBuildV1{}, false, err
	}
	profileSHA, err := profileConfigSHA256(profile)
	if err != nil {
		return ContextMemoryBuildV1{}, false, err
	}
	counter, err := infraai.ResolveTokenCounter(budget.TokenCounterID)
	if err != nil {
		return ContextMemoryBuildV1{}, false, err
	}
	parent, err := repository.latestReadyMemory(ctx, conversationID, profile.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return ContextMemoryBuildV1{}, false, err
	}
	turns, err := memoryTurnsAfterParent(ctx, repository.turns, conversationID, userID, parent)
	if err != nil || len(turns) == 0 {
		return ContextMemoryBuildV1{}, false, err
	}
	selected, build, err := selectMemoryPrefix(turns, counter, budget.KnownInputBudget)
	if err != nil || !build {
		return ContextMemoryBuildV1{}, false, err
	}
	input := MemorySourceInput{ProfileID: profile.ID, ProfileSHA256: profileSHA, ConversationID: conversationID, Turns: selected}
	var previous *uint64
	if parent != nil {
		previous = &parent.ID
		input.ParentMemoryID = previous
		input.ParentSummarySHA256 = parentSummaryHash(parent)
	}
	source, err := MemorySourceSHA256(input)
	if err != nil {
		return ContextMemoryBuildV1{}, false, err
	}
	return ContextMemoryBuildV1{ProfileID: profile.ID, ProfileSHA256: profileSHA, ConversationID: conversationID,
		PreviousMemoryID: previous, FromMessageID: selected[0].UserMessage.ID, ThroughMessageID: selected[len(selected)-1].UserMessage.ID,
		SourceSHA256: source, PolicyVersion: MemoryPolicyVersionV1}, true, nil
}

func (repository *GormMemoryRepository) BuildMemoryBuildPayload(ctx context.Context, conversationID, userID uint64) (ContextMemoryBuildV1, bool, error) {
	return repository.memoryBuildPayload(ctx, conversationID, userID)
}

type memoryBuildBudget struct {
	KnownInputBudget uint64
	TokenCounterID   string
}

func (repository *GormMemoryRepository) memoryBuildAuthority(ctx context.Context, conversationID, userID uint64) (ContextProfile, memoryBuildBudget, error) {
	var profile ContextProfile
	err := repository.db.WithContext(ctx).Table("ai_context_profiles AS p").Select("p.*").
		Joins("JOIN ai_agents AS a ON a.context_profile_id = p.id AND a.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_conversations AS c ON c.agent_id = a.id AND c.id = ? AND c.user_id = ? AND c.is_del = ?", conversationID, userID, enum.CommonNo).
		Where("p.status = ? AND p.memory_provider_model_id IS NOT NULL", ProfileEnabled).Take(&profile).Error
	if err != nil {
		return ContextProfile{}, memoryBuildBudget{}, err
	}
	limits, err := loadMemoryModelLimits(ctx, repository.db, *profile.MemoryProviderModelID)
	return profile, memoryBuildBudget{KnownInputBudget: limits.KnownInputBudget, TokenCounterID: limits.TokenCounterID}, err
}

func (repository *GormMemoryRepository) latestReadyMemory(ctx context.Context, conversationID, profileID uint64) (*MemoryRecord, error) {
	var parent MemoryRecord
	err := repository.db.WithContext(ctx).Where("conversation_id = ? AND context_profile_id_snapshot = ? AND state = ?", conversationID, profileID, MemoryStateReady).
		Order("through_message_id DESC, id DESC").Take(&parent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &parent, err
}

func selectMemoryPrefix(turns []ConversationTurn, counter TokenCounter, knownInputBudget uint64) ([]ConversationTurn, bool, error) {
	tokens := make([]uint64, len(turns))
	var total uint64
	for index, turn := range turns {
		text, err := BuildConversationTurnText(turn, counter, 1<<62)
		if err != nil || text.TokenUpperBound < 0 {
			return nil, false, err
		}
		tokens[index] = uint64(text.TokenUpperBound)
		if ^uint64(0)-total < tokens[index] {
			return nil, false, ErrMemoryInvalid
		}
		total += tokens[index]
	}
	window, build := MemoryWindow(total, knownInputBudget)
	if !build {
		return nil, false, nil
	}
	through, remaining := 0, total
	for through < len(turns) && through < maxConversationTurnPageSize && remaining > window.TargetTokens {
		remaining -= tokens[through]
		through++
	}
	return turns[:through], true, nil
}

func memoryTurnsAfterParent(ctx context.Context, pager ConversationTurnPager, conversationID, userID uint64, parent *MemoryRecord) ([]ConversationTurn, error) {
	if pager == nil {
		return nil, ErrMemoryInvalid
	}
	var before *uint64
	collected := make([]ConversationTurn, 0, maxConversationTurnPageSize)
	for pages := 0; pages < 256; pages++ {
		page, err := pager.PageCompleteBefore(ctx, conversationID, userID, before, maxConversationTurnPageSize)
		if err != nil {
			return nil, err
		}
		collected = append(collected, page.Turns...)
		if len(page.Turns) == 0 || page.NextBeforeUserMessageID == nil {
			break
		}
		if parent != nil && page.Turns[len(page.Turns)-1].UserMessage.ID <= parent.ThroughMessageID {
			break
		}
		before = page.NextBeforeUserMessageID
		if pages == 255 {
			return nil, ErrMemorySnapshotStale
		}
	}
	turns := make([]ConversationTurn, 0, len(collected))
	for index := len(collected) - 1; index >= 0; index-- {
		turn := collected[index]
		if parent == nil || turn.UserMessage.ID > parent.ThroughMessageID {
			turns = append(turns, turn)
		}
	}
	return turns, nil
}
