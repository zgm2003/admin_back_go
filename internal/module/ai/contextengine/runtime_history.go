package contextengine

import (
	"context"
	"fmt"
	"math"

	infraai "admin_back_go/internal/infra/ai"
)

const runtimeHistoryPageSize = 16

func runtimeHistoryGroups(ctx context.Context, pager ConversationTurnPager, input RuntimeInput, facts RuntimeFacts) ([]PackGroup, error) {
	if pager == nil || facts.Budget.KnownInputBudget <= 0 {
		return nil, nil
	}
	counter, err := infraai.ResolveTokenCounter(facts.ModelCapability.TokenCounterID)
	if err != nil {
		return nil, err
	}
	coverage, err := packGroupTokenCoverage(facts.CoreGroups)
	if err != nil || coverage >= facts.Budget.KnownInputBudget {
		return nil, err
	}
	before := input.CurrentMessageID
	groups := make([]PackGroup, 0, runtimeHistoryPageSize)
	for coverage < facts.Budget.KnownInputBudget {
		page, err := pager.PageCompleteBefore(ctx, input.ConversationID, input.UserID, &before, runtimeHistoryPageSize)
		if err != nil {
			return nil, err
		}
		for _, turn := range page.Turns {
			text, err := BuildConversationTurnText(turn, counter, facts.ModelCapability.ContextWindowTokens)
			if err != nil {
				return nil, err
			}
			if turn.UserMessage.ID > math.MaxInt64 {
				return nil, ErrInvalidContextPlan
			}
			ref := fmt.Sprintf("conversation_turn:%d", turn.UserMessage.ID)
			content := text.Text
			groups = append(groups, PackGroup{
				Required: false, Priority: 4, SourceOrder: int64(turn.UserMessage.ID), StableSourceID: ref,
				Blocks: []PackBlock{{Block: ContextBlock{
					Kind: BlockRecentTurn, SourceType: "conversation_turn", SourceRef: ref, SourceSHA256: turn.SourceSHA256,
					AtomicGroupKey: ref, Required: false, Priority: 4, TokenUpperBound: text.TokenUpperBound,
					ContentSnapshot: &content, Metadata: emptyBlockMetadata(),
				}}},
			})
			if text.TokenUpperBound > math.MaxInt64-coverage {
				return nil, ErrInvalidBudget
			}
			coverage += text.TokenUpperBound
			if coverage >= facts.Budget.KnownInputBudget {
				break
			}
		}
		if coverage >= facts.Budget.KnownInputBudget || page.NextBeforeUserMessageID == nil {
			break
		}
		before = *page.NextBeforeUserMessageID
	}
	return groups, nil
}

func packGroupTokenCoverage(groups []PackGroup) (int64, error) {
	var total int64
	for _, group := range groups {
		for _, block := range group.Blocks {
			if block.Block.TokenUpperBound < 0 || block.Block.TokenUpperBound > math.MaxInt64-total {
				return 0, ErrInvalidBudget
			}
			total += block.Block.TokenUpperBound
		}
	}
	return total, nil
}
