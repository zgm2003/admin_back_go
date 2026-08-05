package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
)

type VerifiedCandidate struct {
	Candidate
	SourceType        string
	SourceSHA256      [sha256.Size]byte
	Title             string
	DocumentID        uint64
	DocumentVersionID uint64
	ChunkIDs          []uint64
	ChunkOrdinals     []uint32
	ChunkFactsSHA256  [][sha256.Size]byte
	ContentSHA256     [sha256.Size]byte
	Content           string
	TokenUpperBound   int64
	Locators          []ContextLocatorV1
	ConversationTurn  *ConversationTurn
	RerankScore       *FixedScore
}

func (candidate VerifiedCandidate) CandidateID() string {
	if candidate.SourceType == "conversation_turn" && candidate.ConversationTurn != nil {
		return "conversation_turn:" + strconv.FormatUint(candidate.ConversationTurn.UserMessage.ID, 10)
	}
	parts := make([]string, len(candidate.ChunkIDs))
	for i, id := range candidate.ChunkIDs {
		parts[i] = strconv.FormatUint(id, 10)
	}
	return "document_chunks:" + strings.Join(parts, ",")
}

type CandidateExclusion struct {
	Candidate Candidate
	Reason    ExclusionReason
}

type RetrievalInput struct {
	Collection      string
	Filter          contextindex.ScopeFilter
	Variants        []QueryVariant
	DenseMinScore   *FixedScore
	TopN            uint64
	Authority       CandidateAuthoritySnapshot
	MaxMergedTokens int64
	TokenCounter    TokenCounter
	RerankMinScore  *FixedScore
}

type RetrievalDependencies struct {
	Embedding infraai.EmbeddingClient
	Querier   contextindex.Querier
	Authority CandidateAuthorityReader
	Reranker  infraai.RerankClient
	Now       func() time.Time
}

type RetrievalResult struct {
	Outcome    RetrievalOutcome
	Candidates []VerifiedCandidate
	Excluded   []CandidateExclusion
	Cleanup    []contextindex.PointRef
	Metrics    ContextPlanMetricsV1
}

func Retrieve(ctx context.Context, input RetrievalInput, dependencies RetrievalDependencies) (RetrievalResult, error) {
	result := RetrievalResult{Metrics: ContextPlanMetricsV1{Schema: ContextPlanMetricsSchemaV1}}
	if input.TopN == 0 || input.MaxMergedTokens <= 0 || input.TokenCounter == nil || dependencies.Embedding == nil ||
		dependencies.Querier == nil || dependencies.Authority == nil {
		return result, ErrInvalidContextPlan
	}
	if len(input.Variants) == 0 {
		result.Outcome = RetrievalNoHit
		return result, nil
	}
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	texts := make([]string, len(input.Variants))
	for i, variant := range input.Variants {
		texts[i] = variant.Text
	}
	embeddingStartedAt := now()
	result.Metrics.QueryEmbeddingRequestCount = 1
	embedding, err := dependencies.Embedding.Embed(ctx, texts)
	result.Metrics.QueryEmbeddingMS = elapsedMilliseconds(embeddingStartedAt, now())
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, err)
	}
	if len(embedding.Vectors) != len(input.Variants) {
		cause := fmt.Errorf("%w: embedding vector count disagrees", infraai.ErrEmbeddingFailed)
		result.Outcome = RetrievalFailed
		return result, NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, cause)
	}
	if embedding.Usage.PromptTokens >= 0 && embedding.Usage.TotalTokens >= embedding.Usage.PromptTokens {
		tokens := uint64(embedding.Usage.PromptTokens)
		result.Metrics.QueryInputTokens = &tokens
	}
	denseMin, err := fixedScoreFloat64(input.DenseMinScore)
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, err
	}
	vectors, err := BuildQueryVariantVectors(input.Variants, embedding.Vectors)
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, err
	}
	retrievalStartedAt := now()
	batch, err := dependencies.Querier.QueryBatch(ctx, contextindex.QueryBatchInput{
		Collection: input.Collection, Filter: input.Filter, Variants: vectors, DenseMinScore: denseMin, TopN: input.TopN,
	})
	result.Metrics.RetrievalMS = elapsedMilliseconds(retrievalStartedAt, now())
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, NewEnhancementFailure(EnhancementStageRetrieval, ErrCodeRetrievalFailed, err)
	}
	if uint64(len(batch.Fusion)) > uint64(^uint32(0)) {
		result.Outcome = RetrievalFailed
		return result, ErrInvalidContextPlan
	}
	result.Metrics.CandidateCount = uint32(len(batch.Fusion))
	if len(batch.Fusion) == 0 {
		result.Outcome = RetrievalNoHit
		return result, nil
	}
	candidates, err := CandidatesFromQueryBatch(batch, nil)
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexInconsistent, err)
	}
	authorizationStartedAt := now()
	verification, err := dependencies.Authority.VerifyCandidates(ctx, input.Authority, candidates)
	result.Metrics.AuthorizationMS = elapsedMilliseconds(authorizationStartedAt, now())
	if err != nil {
		result.Outcome = RetrievalFailed
		return result, err
	}
	normalized, excluded, err := NormalizeVerifiedCandidates(verification.Authorized, input.MaxMergedTokens, input.TokenCounter)
	if err != nil {
		result.Outcome = RetrievalFailed
		if errors.Is(err, ErrInvalidContextPlan) {
			return result, NewEnhancementFailure(EnhancementStageIndex, ErrCodeIndexInconsistent, err)
		}
		return result, err
	}
	verification.Excluded = append(verification.Excluded, excluded...)
	if len(normalized) == 0 {
		result.Outcome = RetrievalNoHit
		result.Excluded = verification.Excluded
		result.Cleanup = verification.Cleanup
		return result, nil
	}
	if input.RerankMinScore != nil || dependencies.Reranker != nil {
		rerankStartedAt := now()
		if dependencies.Reranker != nil && input.RerankMinScore != nil {
			result.Metrics.RerankRequestCount = 1
		}
		var rerankTokens *uint64
		normalized, rerankTokens, err = applyRerank(ctx, texts[0], normalized, dependencies.Reranker, input.RerankMinScore)
		result.Metrics.RerankMS = elapsedMilliseconds(rerankStartedAt, now())
		if err != nil {
			result.Outcome = RetrievalFailed
			return result, NewEnhancementFailure(EnhancementStageRerank, ErrCodeRerankFailed, err)
		}
		result.Metrics.RerankInputTokens = rerankTokens
	}
	if len(normalized) == 0 {
		result.Outcome = RetrievalNoHit
		result.Excluded = verification.Excluded
		result.Cleanup = verification.Cleanup
		return result, nil
	}
	result.Outcome = RetrievalHit
	result.Candidates = normalized
	result.Excluded = verification.Excluded
	result.Cleanup = verification.Cleanup
	return result, nil
}

func elapsedMilliseconds(start, end time.Time) uint64 {
	if !end.After(start) {
		return 0
	}
	return uint64(end.Sub(start) / time.Millisecond)
}

func fixedScoreFloat64(score *FixedScore) (*float64, error) {
	if score == nil {
		return nil, nil
	}
	if err := score.Validate(); err != nil {
		return nil, err
	}
	value, err := strconv.ParseFloat(score.String(), 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func NormalizeVerifiedCandidates(input []VerifiedCandidate, maxMergedTokens int64, counter TokenCounter) ([]VerifiedCandidate, []CandidateExclusion, error) {
	if maxMergedTokens <= 0 || counter == nil {
		return nil, nil, ErrInvalidBudget
	}
	separatorBound, err := counter.UpperBoundText("\n\n")
	if err != nil || separatorBound < 0 {
		return nil, nil, ErrInvalidBudget
	}
	seenDocuments := make(map[[sha256.Size]byte]struct{}, len(input))
	seenTurns := make(map[[sha256.Size]byte]struct{}, len(input))
	deduplicated := make([]VerifiedCandidate, 0, len(input))
	excluded := make([]CandidateExclusion, 0)
	for _, candidate := range input {
		if err := validateVerifiedCandidate(candidate); err != nil {
			return nil, nil, err
		}
		key := candidate.ContentSHA256
		seen := seenDocuments
		if candidate.SourceType == "conversation_turn" {
			key = candidate.SourceSHA256
			seen = seenTurns
		}
		if _, duplicate := seen[key]; duplicate {
			excluded = append(excluded, CandidateExclusion{Candidate: candidate.Candidate, Reason: ExclusionDuplicateContent})
			continue
		}
		seen[key] = struct{}{}
		deduplicated = append(deduplicated, cloneVerifiedCandidate(candidate))
	}

	merged := make([]VerifiedCandidate, 0, len(deduplicated))
	for _, candidate := range deduplicated {
		if len(merged) == 0 || !canMergeCandidates(merged[len(merged)-1], candidate, maxMergedTokens, separatorBound) {
			merged = append(merged, candidate)
			continue
		}
		combined, err := mergeCandidates(merged[len(merged)-1], candidate, separatorBound)
		if err != nil {
			return nil, nil, err
		}
		merged[len(merged)-1] = combined
	}
	return merged, excluded, nil
}

func ApplyRerank(
	ctx context.Context,
	query string,
	candidates []VerifiedCandidate,
	client infraai.RerankClient,
	minScore *FixedScore,
) ([]VerifiedCandidate, error) {
	result, _, err := applyRerank(ctx, query, candidates, client, minScore)
	return result, err
}

func applyRerank(
	ctx context.Context,
	query string,
	candidates []VerifiedCandidate,
	client infraai.RerankClient,
	minScore *FixedScore,
) ([]VerifiedCandidate, *uint64, error) {
	if client == nil && minScore == nil {
		return cloneVerifiedCandidates(candidates), nil, nil
	}
	if client == nil || minScore == nil || minScore.Validate() != nil || strings.TrimSpace(query) == "" {
		return nil, nil, fmt.Errorf("%w: rerank policy is incomplete", infraai.ErrRerankFailed)
	}
	if len(candidates) == 0 {
		return []VerifiedCandidate{}, nil, nil
	}
	documents := make([]infraai.RerankDocument, len(candidates))
	byID := make(map[string]int, len(candidates))
	for i, candidate := range candidates {
		if err := validateVerifiedCandidate(candidate); err != nil {
			return nil, nil, fmt.Errorf("%w: candidate facts are invalid", infraai.ErrRerankFailed)
		}
		id := candidate.CandidateID()
		if id == "document_chunks:" || strings.TrimSpace(candidate.Content) == "" {
			return nil, nil, fmt.Errorf("%w: candidate is not rerankable", infraai.ErrRerankFailed)
		}
		if _, duplicate := byID[id]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate candidate identity", infraai.ErrRerankFailed)
		}
		byID[id] = i
		documents[i] = infraai.RerankDocument{CandidateID: id, Text: candidate.Content}
	}
	result, err := client.Rerank(ctx, query, documents)
	if err != nil {
		if errors.Is(err, infraai.ErrRerankFailed) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: provider request failed", infraai.ErrRerankFailed)
	}
	if len(result.Scores) != len(candidates) {
		return nil, nil, fmt.Errorf("%w: provider result count disagrees", infraai.ErrRerankFailed)
	}
	type rankedCandidate struct {
		candidate VerifiedCandidate
		original  int
		score     FixedScore
	}
	ranked := make([]rankedCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, item := range result.Scores {
		original, ok := byID[item.CandidateID]
		if !ok || item.Score < 0 || item.Score > 1 {
			return nil, nil, fmt.Errorf("%w: provider returned an unknown candidate", infraai.ErrRerankFailed)
		}
		if _, duplicate := seen[item.CandidateID]; duplicate {
			return nil, nil, fmt.Errorf("%w: provider duplicated a candidate", infraai.ErrRerankFailed)
		}
		seen[item.CandidateID] = struct{}{}
		score, scoreErr := FixedScoreFromFloat64(item.Score)
		if scoreErr != nil {
			return nil, nil, fmt.Errorf("%w: provider score is invalid", infraai.ErrRerankFailed)
		}
		comparison, compareErr := score.Compare(*minScore)
		if compareErr != nil {
			return nil, nil, fmt.Errorf("%w: compare provider score", infraai.ErrRerankFailed)
		}
		if comparison < 0 {
			continue
		}
		candidate := cloneVerifiedCandidate(candidates[original])
		candidate.RerankScore = &score
		ranked = append(ranked, rankedCandidate{candidate: candidate, original: original, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		comparison, _ := ranked[i].score.Compare(ranked[j].score)
		if comparison != 0 {
			return comparison > 0
		}
		return ranked[i].original < ranked[j].original
	})
	output := make([]VerifiedCandidate, len(ranked))
	for i := range ranked {
		output[i] = ranked[i].candidate
	}
	var inputTokens *uint64
	if result.Usage.TotalTokens >= 0 {
		tokens := uint64(result.Usage.TotalTokens)
		inputTokens = &tokens
	}
	return output, inputTokens, nil
}

func validateVerifiedCandidate(candidate VerifiedCandidate) error {
	if err := candidate.Point.Validate(); err != nil {
		return err
	}
	if candidate.FusionScore.Validate() != nil || candidate.Branches.Validate() != nil ||
		candidate.SourceSHA256 == ([sha256.Size]byte{}) || strings.TrimSpace(candidate.Content) == "" || candidate.TokenUpperBound <= 0 {
		return ErrInvalidContextPlan
	}
	switch candidate.SourceType {
	case "document_chunk":
		if candidate.DocumentID == 0 || candidate.DocumentVersionID == 0 || len(candidate.ChunkIDs) == 0 ||
			strings.TrimSpace(candidate.Title) == "" ||
			len(candidate.ChunkIDs) != len(candidate.ChunkOrdinals) || len(candidate.ChunkIDs) != len(candidate.ChunkFactsSHA256) ||
			len(candidate.ChunkIDs) != len(candidate.Locators) || candidate.ContentSHA256 == ([sha256.Size]byte{}) {
			return ErrInvalidContextPlan
		}
	case "conversation_turn":
		if candidate.ConversationTurn == nil || len(candidate.ChunkIDs) != 0 || candidate.SourceSHA256 != candidate.ConversationTurn.SourceSHA256 {
			return ErrInvalidContextPlan
		}
	default:
		return ErrInvalidContextPlan
	}
	return nil
}

func canMergeCandidates(left, right VerifiedCandidate, maxMergedTokens, separatorBound int64) bool {
	if left.SourceType != "document_chunk" || right.SourceType != "document_chunk" ||
		left.DocumentID != right.DocumentID || left.DocumentVersionID != right.DocumentVersionID ||
		len(left.ChunkOrdinals) == 0 || len(right.ChunkOrdinals) != 1 || len(left.Locators) == 0 || len(right.Locators) != 1 ||
		left.ChunkOrdinals[len(left.ChunkOrdinals)-1]+1 != right.ChunkOrdinals[0] ||
		left.TokenUpperBound+separatorBound+right.TokenUpperBound > maxMergedTokens {
		return false
	}
	return adjacentLocator(left.Locators[len(left.Locators)-1], right.Locators[0])
}

func adjacentLocator(left, right ContextLocatorV1) bool {
	if left.Kind != right.Kind || !slices.Equal(left.HeadingPath, right.HeadingPath) {
		return false
	}
	if left.Paragraph != nil && right.Paragraph != nil {
		return equalOptionalUint32(left.Page, right.Page) && equalOptionalString(left.Sheet, right.Sheet) && *left.Paragraph+1 == *right.Paragraph
	}
	if left.LineEnd != nil && right.LineStart != nil {
		return equalOptionalUint32(left.Page, right.Page) && equalOptionalString(left.Sheet, right.Sheet) && *left.LineEnd+1 == *right.LineStart
	}
	if left.RowEnd != nil && right.RowStart != nil && equalOptionalString(left.Sheet, right.Sheet) {
		return *left.RowEnd+1 == *right.RowStart
	}
	return false
}

func mergeCandidates(left, right VerifiedCandidate, separatorBound int64) (VerifiedCandidate, error) {
	merged := cloneVerifiedCandidate(left)
	merged.ChunkIDs = append(merged.ChunkIDs, right.ChunkIDs...)
	merged.ChunkOrdinals = append(merged.ChunkOrdinals, right.ChunkOrdinals...)
	merged.ChunkFactsSHA256 = append(merged.ChunkFactsSHA256, right.ChunkFactsSHA256...)
	merged.Locators = append(merged.Locators, right.Locators...)
	merged.Content += "\n\n" + right.Content
	merged.ContentSHA256 = sha256.Sum256([]byte(merged.Content))
	merged.TokenUpperBound += right.TokenUpperBound + separatorBound
	mergedHash, err := documentChunkSourceSHA256(merged.ChunkIDs, merged.ChunkFactsSHA256)
	if err != nil {
		return VerifiedCandidate{}, err
	}
	merged.SourceSHA256 = mergedHash
	merged.Branches.Branches = append(merged.Branches.Branches, right.Branches.Branches...)
	return merged, nil
}

func documentChunkSourceSHA256(chunkIDs []uint64, chunkHashes [][sha256.Size]byte) ([sha256.Size]byte, error) {
	if len(chunkIDs) == 0 || len(chunkIDs) != len(chunkHashes) {
		return [sha256.Size]byte{}, ErrInvalidContextPlan
	}
	seen := make(map[uint64]struct{}, len(chunkIDs))
	for index, id := range chunkIDs {
		if id == 0 || isZeroSHA256(chunkHashes[index]) {
			return [sha256.Size]byte{}, ErrInvalidContextPlan
		}
		if _, duplicate := seen[id]; duplicate {
			return [sha256.Size]byte{}, ErrInvalidContextPlan
		}
		seen[id] = struct{}{}
	}
	if len(chunkIDs) == 1 {
		return chunkHashes[0], nil
	}
	type mergedChunkHash struct {
		ChunkID uint64 `json:"chunk_id"`
		SHA256  string `json:"chunk_facts_sha256"`
	}
	hashInput := make([]mergedChunkHash, len(chunkIDs))
	for index, id := range chunkIDs {
		hashInput[index] = mergedChunkHash{ChunkID: id, SHA256: fmt.Sprintf("%x", chunkHashes[index])}
	}
	raw, err := json.Marshal(struct {
		Schema string            `json:"schema"`
		Chunks []mergedChunkHash `json:"chunks"`
	}{Schema: "merged_document_chunks_v1", Chunks: hashInput})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func cloneVerifiedCandidates(input []VerifiedCandidate) []VerifiedCandidate {
	result := make([]VerifiedCandidate, len(input))
	for i := range input {
		result[i] = cloneVerifiedCandidate(input[i])
	}
	return result
}

func cloneVerifiedCandidate(input VerifiedCandidate) VerifiedCandidate {
	cloned := input
	cloned.ChunkIDs = slices.Clone(input.ChunkIDs)
	cloned.ChunkOrdinals = slices.Clone(input.ChunkOrdinals)
	cloned.ChunkFactsSHA256 = slices.Clone(input.ChunkFactsSHA256)
	cloned.Locators = slices.Clone(input.Locators)
	cloned.Branches.Branches = slices.Clone(input.Branches.Branches)
	if input.ConversationTurn != nil {
		turn := *input.ConversationTurn
		cloned.ConversationTurn = &turn
	}
	if input.RerankScore != nil {
		score := *input.RerankScore
		cloned.RerankScore = &score
	}
	return cloned
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalUint32(left, right *uint32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
