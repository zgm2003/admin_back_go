package contextengine

import (
	"errors"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"admin_back_go/internal/shared/apperror"
)

type PackInput struct {
	KnownInputBudget                int64
	ToolContinuationInputReserve    int64
	ToolContinuationInputUpperBound int64
	Candidates                      []PackGroup
}

type PackGroup struct {
	Required       bool
	Priority       int32
	Relevance      *FixedScore
	SourceOrder    int64
	StableSourceID string
	Blocks         []PackBlock
}

type PackBlock struct {
	Block       ContextBlock
	FusionScore *FixedScore
	RerankScore *FixedScore
}

type PackedContext struct {
	KnownInputUpperBound int64
	Items                []ContextPlanItem
}

type rankedPackGroup struct {
	group       PackGroup
	relevance   *big.Rat
	tokens      int64
	sumOverflow bool
	selected    bool
}

func Pack(input PackInput) (PackedContext, *apperror.Error) {
	groups, appErr := validateAndRankPackInput(input)
	if appErr != nil {
		return PackedContext{}, appErr
	}
	if input.ToolContinuationInputUpperBound > input.ToolContinuationInputReserve {
		return PackedContext{}, contextPackError(ErrCodeToolContinuationOverflow, nil)
	}

	var requiredTokens int64
	for index := range groups {
		if !groups[index].group.Required {
			continue
		}
		if groups[index].sumOverflow || groups[index].tokens > input.KnownInputBudget-requiredTokens {
			return PackedContext{}, contextPackError(ErrCodeRequiredOverflow, nil)
		}
		requiredTokens += groups[index].tokens
		groups[index].selected = true
	}

	remainingOptionalBudget := input.KnownInputBudget - requiredTokens
	selectedTokens := requiredTokens
	for index := range groups {
		if groups[index].group.Required || groups[index].sumOverflow || groups[index].tokens > remainingOptionalBudget {
			continue
		}
		groups[index].selected = true
		remainingOptionalBudget -= groups[index].tokens
		selectedTokens += groups[index].tokens
	}

	items := make([]ContextPlanItem, 0, countPackBlocks(groups))
	nextCitation := 1
	for _, ranked := range groups {
		for _, candidate := range ranked.group.Blocks {
			block := cloneContextBlock(candidate.Block)
			block.AtomicGroupKey = ranked.group.StableSourceID
			block.Required = ranked.group.Required
			block.Priority = ranked.group.Priority
			item := ContextPlanItem{
				Ordinal:     uint32(len(items) + 1),
				Block:       block,
				FusionScore: clonePointer(candidate.FusionScore),
				RerankScore: clonePointer(candidate.RerankScore),
			}
			if ranked.selected {
				item.Decision = DecisionSelected
				if block.Kind == BlockDocumentEvidence {
					citation := "C" + strconv.Itoa(nextCitation)
					item.CitationKey = &citation
					nextCitation++
				}
			} else {
				item.Decision = DecisionExcluded
				reason := ExclusionBudgetExceeded
				item.ExclusionReason = &reason
				item.Block.ContentSnapshot = nil
			}
			items = append(items, item)
		}
	}

	return PackedContext{KnownInputUpperBound: selectedTokens, Items: items}, nil
}

func validateAndRankPackInput(input PackInput) ([]rankedPackGroup, *apperror.Error) {
	if input.KnownInputBudget < 0 || input.ToolContinuationInputReserve < 0 ||
		input.ToolContinuationInputUpperBound < 0 || len(input.Candidates) == 0 {
		return nil, invalidPackInput(errors.New("invalid pack budget or empty candidates"))
	}

	groups := make([]rankedPackGroup, 0, len(input.Candidates))
	seenIDs := make(map[string]struct{}, len(input.Candidates))
	totalBlocks := uint64(0)
	for _, group := range input.Candidates {
		if strings.TrimSpace(group.StableSourceID) == "" || strings.TrimSpace(group.StableSourceID) != group.StableSourceID ||
			group.Priority <= 0 || len(group.Blocks) == 0 {
			return nil, invalidPackInput(errors.New("invalid atomic group facts"))
		}
		if _, exists := seenIDs[group.StableSourceID]; exists {
			return nil, invalidPackInput(errors.New("duplicate stable source ID"))
		}
		seenIDs[group.StableSourceID] = struct{}{}

		ranked := rankedPackGroup{group: group}
		if group.Relevance != nil {
			if err := group.Relevance.Validate(); err != nil {
				return nil, invalidPackInput(err)
			}
			var ok bool
			ranked.relevance, ok = new(big.Rat).SetString(group.Relevance.String())
			if !ok {
				return nil, invalidPackInput(ErrInvalidFixedScore)
			}
		}
		for _, block := range group.Blocks {
			if err := validatePackBlock(block); err != nil {
				return nil, invalidPackInput(err)
			}
			if ranked.sumOverflow {
				continue
			}
			if block.Block.TokenUpperBound > math.MaxInt64-ranked.tokens {
				ranked.sumOverflow = true
				continue
			}
			ranked.tokens += block.Block.TokenUpperBound
		}
		totalBlocks += uint64(len(group.Blocks))
		if totalBlocks > math.MaxUint32 {
			return nil, invalidPackInput(errors.New("too many context blocks"))
		}
		groups = append(groups, ranked)
	}

	sort.Slice(groups, func(left, right int) bool {
		if groups[left].group.Priority != groups[right].group.Priority {
			return groups[left].group.Priority < groups[right].group.Priority
		}
		leftScore, rightScore := groups[left].relevance, groups[right].relevance
		if leftScore != nil || rightScore != nil {
			if leftScore == nil {
				return false
			}
			if rightScore == nil {
				return true
			}
			if compared := leftScore.Cmp(rightScore); compared != 0 {
				return compared > 0
			}
		}
		if groups[left].group.SourceOrder != groups[right].group.SourceOrder {
			return groups[left].group.SourceOrder > groups[right].group.SourceOrder
		}
		return groups[left].group.StableSourceID < groups[right].group.StableSourceID
	})
	return groups, nil
}

func validatePackBlock(candidate PackBlock) error {
	block := candidate.Block
	if block.TokenUpperBound < 0 || !sourceTypePattern.MatchString(block.SourceType) ||
		strings.TrimSpace(block.SourceRef) == "" || isZeroSHA256(block.SourceSHA256) {
		return ErrInvalidContextPlan
	}
	if err := block.Kind.Validate(); err != nil {
		return err
	}
	if err := block.Metadata.Validate(); err != nil {
		return err
	}
	if block.Kind.isAttachment() != (block.Metadata.Attachment != nil) {
		return ErrInvalidContextPlan
	}
	if block.Kind.isAttachment() {
		if block.ContentSnapshot != nil {
			return ErrInvalidContextPlan
		}
	} else if block.ContentSnapshot == nil || strings.TrimSpace(*block.ContentSnapshot) == "" {
		return ErrInvalidContextPlan
	}
	for _, score := range []*FixedScore{candidate.FusionScore, candidate.RerankScore} {
		if score != nil {
			if err := score.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func countPackBlocks(groups []rankedPackGroup) int {
	count := 0
	for _, group := range groups {
		count += len(group.group.Blocks)
	}
	return count
}

func cloneContextBlock(block ContextBlock) ContextBlock {
	cloned := block
	cloned.ContentSnapshot = clonePointer(block.ContentSnapshot)
	cloned.Metadata = cloneContextBlockMetadata(block.Metadata)
	return cloned
}

func cloneContextBlockMetadata(metadata ContextBlockMetadataV1) ContextBlockMetadataV1 {
	cloned := metadata
	if metadata.Attachment != nil {
		attachment := *metadata.Attachment
		cloned.Attachment = &attachment
	}
	if metadata.Locator != nil {
		locator := *metadata.Locator
		locator.Page = clonePointer(metadata.Locator.Page)
		locator.Paragraph = clonePointer(metadata.Locator.Paragraph)
		locator.LineStart = clonePointer(metadata.Locator.LineStart)
		locator.LineEnd = clonePointer(metadata.Locator.LineEnd)
		locator.RowStart = clonePointer(metadata.Locator.RowStart)
		locator.RowEnd = clonePointer(metadata.Locator.RowEnd)
		locator.Sheet = clonePointer(metadata.Locator.Sheet)
		locator.CellStart = clonePointer(metadata.Locator.CellStart)
		locator.CellEnd = clonePointer(metadata.Locator.CellEnd)
		locator.HeadingPath = append([]string(nil), metadata.Locator.HeadingPath...)
		cloned.Locator = &locator
	}
	if metadata.Retrieval != nil {
		retrieval := *metadata.Retrieval
		retrieval.Branches = append([]RetrievalBranchV1(nil), metadata.Retrieval.Branches...)
		cloned.Retrieval = &retrieval
	}
	if metadata.Document != nil {
		document := *metadata.Document
		document.ChunkIDs = append([]uint64(nil), metadata.Document.ChunkIDs...)
		document.Locators = make([]ContextLocatorV1, len(metadata.Document.Locators))
		for index, source := range metadata.Document.Locators {
			locator := source
			locator.Page = clonePointer(source.Page)
			locator.Paragraph = clonePointer(source.Paragraph)
			locator.LineStart = clonePointer(source.LineStart)
			locator.LineEnd = clonePointer(source.LineEnd)
			locator.RowStart = clonePointer(source.RowStart)
			locator.RowEnd = clonePointer(source.RowEnd)
			locator.Sheet = clonePointer(source.Sheet)
			locator.CellStart = clonePointer(source.CellStart)
			locator.CellEnd = clonePointer(source.CellEnd)
			locator.HeadingPath = append([]string(nil), source.HeadingPath...)
			document.Locators[index] = locator
		}
		cloned.Document = &document
	}
	return cloned
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func contextPackError(code ErrorCode, cause error) *apperror.Error {
	appErr, err := NewContextAppError(code, cause)
	if err != nil {
		return invalidPackInput(err)
	}
	return appErr
}

func invalidPackInput(cause error) *apperror.Error {
	return apperror.Wrap(
		"internal.unknown", apperror.CategoryInternal, http.StatusInternalServerError, apperror.Permanent,
		"", nil, "上下文装配输入无效", cause,
	)
}
