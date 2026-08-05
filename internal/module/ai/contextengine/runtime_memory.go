package contextengine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

type MemoryContextReader interface {
	LatestReadyMemory(context.Context, uint64, uint64, [sha256.Size]byte) (*MemoryRecord, error)
}

type RuntimeMemoryContext struct {
	Record   *MemoryRecord
	Expected bool
}

func (materializer *PlanMaterializer) runtimeMemory(ctx context.Context, input RuntimeInput, facts RuntimeFacts) (RuntimeMemoryContext, error) {
	if facts.Profile == nil {
		return RuntimeMemoryContext{}, nil
	}
	if materializer == nil || facts.Retrieval == nil || facts.Retrieval.Profile.ID != facts.Profile.ID {
		return RuntimeMemoryContext{}, ErrInvalidContextPlan
	}
	if facts.Retrieval.Profile.MemoryProviderModelID == nil {
		return RuntimeMemoryContext{}, nil
	}
	if *facts.Retrieval.Profile.MemoryProviderModelID == 0 || materializer.memory == nil || materializer.history == nil {
		return RuntimeMemoryContext{}, ErrMemoryInvalid
	}
	memory, err := materializer.memory.LatestReadyMemory(ctx, input.ConversationID, facts.Profile.ID, facts.Profile.SHA256)
	if err != nil {
		return RuntimeMemoryContext{}, err
	}
	if memory != nil {
		if memory.ConversationID != input.ConversationID || memory.ProfileID != facts.Profile.ID || !readyMemoryRowValid(*memory, facts.Profile.SHA256) {
			return RuntimeMemoryContext{Expected: true}, nil
		}
		return RuntimeMemoryContext{Record: memory, Expected: true}, nil
	}
	if facts.Budget.KnownInputBudget < 0 {
		return RuntimeMemoryContext{}, ErrInvalidBudget
	}
	counter, err := infraai.ResolveTokenCounter(facts.ModelCapability.TokenCounterID)
	if err != nil {
		return RuntimeMemoryContext{}, err
	}
	turns, err := memoryTurnsAfterParent(ctx, materializer.history, input.ConversationID, input.UserID, nil)
	if err != nil {
		return RuntimeMemoryContext{}, err
	}
	_, total, err := memoryTurnTokens(turns, counter)
	if err != nil {
		return RuntimeMemoryContext{}, err
	}
	_, expected := MemoryWindow(total, uint64(facts.Budget.KnownInputBudget))
	return RuntimeMemoryContext{Expected: expected}, nil
}

func memoryPackGroup(memory MemoryRecord, counterID string) (PackGroup, error) {
	if memory.ID == 0 || memory.Summary == nil || strings.TrimSpace(*memory.Summary) == "" {
		return PackGroup{}, ErrInvalidContextPlan
	}
	counter, err := infraai.ResolveTokenCounter(counterID)
	if err != nil {
		return PackGroup{}, err
	}
	sourceHash, err := SHA256FromBytes(memory.SourceSHA256)
	if err != nil {
		return PackGroup{}, err
	}
	content := "[CONVERSATION_MEMORY]\n" + *memory.Summary + "\n[/CONVERSATION_MEMORY]"
	bound, err := counter.UpperBoundText(content)
	if err != nil || bound <= 0 {
		return PackGroup{}, ErrInvalidContextPlan
	}
	ref := "conversation_memory:" + strconv.FormatUint(memory.ID, 10)
	return PackGroup{Priority: 6, SourceOrder: int64(memory.ThroughMessageID), StableSourceID: ref, Blocks: []PackBlock{{Block: ContextBlock{
		Kind: BlockConversationMemory, SourceType: "conversation_memory", SourceRef: ref, SourceSHA256: sourceHash,
		AtomicGroupKey: ref, Priority: 6, TokenUpperBound: bound, ContentSnapshot: &content, Metadata: emptyBlockMetadata(),
	}}}}, nil
}

func composeEvidenceGroups(history, evidence []PackGroup, memoryBoundary uint64) []PackGroup {
	seenTurns := make(map[[sha256.Size]byte]struct{}, len(history))
	seenIDs := make(map[string]struct{}, len(history)+len(evidence))
	for _, group := range history {
		seenIDs[group.StableSourceID] = struct{}{}
		for _, block := range group.Blocks {
			if block.Block.SourceType == "conversation_turn" {
				seenTurns[block.Block.SourceSHA256] = struct{}{}
			}
		}
	}
	result := clonePackGroups(evidence)
	for index := range result {
		group := &result[index]
		if len(group.Blocks) == 0 {
			continue
		}
		block := &group.Blocks[0].Block
		switch block.SourceType {
		case "document_chunk":
			group.Priority = 5
		case "conversation_turn":
			group.Priority = 8
			block.Kind = BlockRecalledTurn
			anchor, err := parseAuthorityID(block.SourceRef, "conversation_turn:")
			if err == nil && memoryBoundary != 0 && anchor <= memoryBoundary {
				reason := ExclusionSupersededMemory
				group.ExcludedReason = &reason
			} else if _, duplicate := seenTurns[block.SourceSHA256]; duplicate {
				reason := ExclusionDuplicateContent
				group.ExcludedReason = &reason
			}
			seenTurns[block.SourceSHA256] = struct{}{}
		}
		for blockIndex := range group.Blocks {
			group.Blocks[blockIndex].Block.Priority = group.Priority
		}
		if _, duplicate := seenIDs[group.StableSourceID]; duplicate {
			group.StableSourceID = fmt.Sprintf("retrieved:%d:%s", index, group.StableSourceID)
		}
		seenIDs[group.StableSourceID] = struct{}{}
	}
	return result
}
