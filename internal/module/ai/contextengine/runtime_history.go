package contextengine

import (
	"context"
	"errors"
	"fmt"
	"math"

	infraai "admin_back_go/internal/infra/ai"
)

const runtimeHistoryPageSize = 16

var ErrAttachmentUnavailable = errors.New(string(ErrCodeAttachmentUnavailable))

func runtimeHistoryGroups(ctx context.Context, pager ConversationTurnPager, availability HistoricalAttachmentAvailability, input RuntimeInput, facts RuntimeFacts) ([]PackGroup, error) {
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
	pageNumber := 0
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
			blocks := []PackBlock{{Block: ContextBlock{
				Kind: BlockRecentTurn, SourceType: "conversation_turn", SourceRef: ref, SourceSHA256: turn.SourceSHA256,
				AtomicGroupKey: ref, Required: false, Priority: 4, TokenUpperBound: text.TokenUpperBound,
				ContentSnapshot: &content, Metadata: emptyBlockMetadata(),
			}}}
			for _, attachment := range turn.UserMessage.Attachments {
				if pageNumber > 0 {
					if availability == nil {
						return nil, ErrAttachmentUnavailable
					}
					ready, err := availability.HistoricalAttachmentReady(ctx, turn.ConversationID, turn.UserMessage.ID, attachment.Index)
					if err != nil {
						return nil, err
					}
					if !ready {
						return nil, ErrAttachmentUnavailable
					}
					continue
				}
				block, err := nativeHistoryAttachmentBlock(turn, attachment, ref)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, block)
			}
			groups = append(groups, PackGroup{
				Required: false, Priority: 4, SourceOrder: int64(turn.UserMessage.ID), StableSourceID: ref,
				Blocks: blocks,
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
		pageNumber++
	}
	return groups, nil
}

func nativeHistoryAttachmentBlock(turn ConversationTurn, attachment TurnAttachment, groupKey string) (PackBlock, error) {
	kind := AttachmentKind(attachment.Type)
	metadata := emptyBlockMetadata()
	metadata.Attachment = &ContextAttachmentV1{
		Kind: kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Name,
	}
	if err := metadata.Attachment.Validate(); err != nil {
		return PackBlock{}, ErrAttachmentUnavailable
	}
	facts := runtimeAttachment{
		Kind: kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Name,
	}
	hash, err := hashRuntimeFacts(facts)
	if err != nil {
		return PackBlock{}, err
	}
	return PackBlock{Block: ContextBlock{
		Kind: BlockHistoryAttachment, SourceType: "attachment",
		SourceRef: fmt.Sprintf("message:%d/attachment:%d", turn.UserMessage.ID, attachment.Index), SourceSHA256: hash,
		AtomicGroupKey: groupKey, Required: false, Priority: 4, TokenUpperBound: 0, Metadata: metadata,
	}}, nil
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
