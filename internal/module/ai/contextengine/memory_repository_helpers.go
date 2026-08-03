package contextengine

import (
	"context"
	"crypto/sha256"
	"strings"
)

func validateMemoryRangeContinuity(ctx context.Context, pager ConversationTurnPager, conversationID, userID, fromMessageID uint64, parent *MemoryRecord) error {
	if pager == nil || fromMessageID == 0 {
		return ErrMemoryInvalid
	}
	before := fromMessageID
	page, err := pager.PageCompleteBefore(ctx, conversationID, userID, &before, 1)
	if err != nil {
		return err
	}
	if parent == nil {
		if len(page.Turns) != 0 {
			return ErrMemoryParentGap
		}
		return nil
	}
	if len(page.Turns) != 1 || page.Turns[0].UserMessage.ID != parent.ThroughMessageID {
		return ErrMemoryParentGap
	}
	return nil
}

func memoryTurnsForRange(ctx context.Context, pager ConversationTurnPager, conversationID, userID, fromMessageID, throughMessageID uint64) ([]ConversationTurn, error) {
	if pager == nil || conversationID == 0 || userID == 0 || fromMessageID == 0 || throughMessageID < fromMessageID {
		return nil, ErrMemoryInvalid
	}
	var before *uint64
	if throughMessageID < ^uint64(0) {
		value := throughMessageID + 1
		before = &value
	}
	page, err := pager.PageCompleteBefore(ctx, conversationID, userID, before, maxConversationTurnPageSize)
	if err != nil {
		return nil, err
	}
	selected := make([]ConversationTurn, 0, len(page.Turns))
	for index := len(page.Turns) - 1; index >= 0; index-- {
		turn := page.Turns[index]
		if turn.UserMessage.ID >= fromMessageID && turn.UserMessage.ID <= throughMessageID {
			selected = append(selected, turn)
		}
	}
	if len(selected) == 0 || selected[0].UserMessage.ID != fromMessageID || selected[len(selected)-1].UserMessage.ID != throughMessageID {
		return nil, ErrMemorySnapshotStale
	}
	return selected, nil
}

func memoryProfileSHA256(profile ContextProfile) ([sha256.Size]byte, error) {
	dense, err := ParseFixedScore(profile.DenseMinScore)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	var reranker *FixedScore
	if profile.RerankerMinScore != nil {
		value, err := ParseFixedScore(*profile.RerankerMinScore)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		reranker = &value
	}
	return HashContextProfile(ContextProfileHashInput{
		EmbeddingProviderModelID: profile.EmbeddingProviderModelID, EmbeddingDimensions: profile.EmbeddingDimensions,
		EmbeddingMaxInputTokens: profile.EmbeddingMaxInputTokens, EmbeddingTokenCounterID: profile.EmbeddingTokenCounterID,
		DenseDistance: DenseDistance(profile.DenseDistance), DenseMinScore: dense,
		SparseEncoder: profile.SparseEncoder, SparseEncoderVersion: profile.SparseEncoderVersion,
		RerankerProviderModelID: profile.RerankerProviderModelID, RerankerMinScore: reranker,
		MemoryProviderModelID: profile.MemoryProviderModelID,
	})
}

func parentSummaryHash(parent *MemoryRecord) [sha256.Size]byte {
	if parent == nil || parent.Summary == nil {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256([]byte(*parent.Summary))
}

func memoryRowFromCandidate(candidate MemoryCandidate) MemoryRecord {
	row := MemoryRecord{ConversationID: candidate.ConversationID, ProfileID: candidate.ProfileID,
		ProfileSHA256: candidate.ProfileSHA256[:], ParentMemoryID: candidate.ParentMemoryID,
		FromMessageID: candidate.FromMessageID, ThroughMessageID: candidate.ThroughMessageID,
		SourceSHA256: candidate.SourceSHA256[:], PolicyVersion: candidate.PolicyVersion, State: candidate.State}
	if candidate.State == MemoryStateReady {
		summary, requestID := candidate.Summary, strings.TrimSpace(candidate.ProviderRequestID)
		row.Summary, row.SummarySHA256 = &summary, candidate.SummarySHA256[:]
		row.PromptTokens, row.CompletionTokens = candidate.PromptTokens, candidate.CompletionTokens
		if requestID != "" {
			row.ProviderRequestID = &requestID
		}
	} else {
		code := strings.TrimSpace(candidate.ErrorCode)
		row.ErrorCode = &code
	}
	return row
}
