package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"admin_back_go/internal/infra/contextindex"

	"golang.org/x/text/unicode/norm"
)

type QueryVariant struct {
	VariantID   string
	Text        string
	QuerySHA256 [32]byte
	Sparse      contextindex.SparseVector
}

type Candidate struct {
	Point       contextindex.PointRef
	FusionScore FixedScore
	Branches    RetrievalBranchesV1
}

func BuildQueryVariants(currentText string, newest *ConversationTurn, counter TokenCounter, maxTurnTokens int64) ([]QueryVariant, error) {
	current, err := normalizeQueryText(currentText)
	if err != nil {
		return nil, err
	}
	if current == "" {
		return nil, nil
	}
	texts := []struct {
		id   string
		text string
	}{{id: "current", text: current}}
	if newest != nil {
		turnText, turnErr := BuildConversationTurnText(*newest, counter, maxTurnTokens)
		if turnErr != nil {
			return nil, turnErr
		}
		if turnText.Text != "" {
			texts = append(texts, struct {
				id   string
				text string
			}{id: "current_with_newest_turn", text: current + "\n\n" + strings.TrimSuffix(turnText.Text, "\n")})
		}
	}
	seen := make(map[[32]byte]struct{}, len(texts))
	variants := make([]QueryVariant, 0, len(texts))
	for _, item := range texts {
		hash, hashErr := CanonicalQuerySHA256(item.text)
		if hashErr != nil {
			return nil, hashErr
		}
		if _, duplicate := seen[hash]; duplicate {
			continue
		}
		sparse, sparseErr := EncodeSparse(item.text)
		if sparseErr != nil {
			return nil, sparseErr
		}
		seen[hash] = struct{}{}
		variants = append(variants, QueryVariant{VariantID: item.id, Text: item.text, QuerySHA256: hash, Sparse: sparse})
	}
	return variants, nil
}

func CanonicalQuerySHA256(text string) ([32]byte, error) {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return [32]byte{}, errors.New("canonical query text must be non-empty UTF-8")
	}
	raw, err := json.Marshal(struct {
		Schema string `json:"schema"`
		Text   string `json:"text"`
	}{Schema: "context_query_v1", Text: text})
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func BuildQueryVariantVectors(variants []QueryVariant, dense [][]float32) ([]contextindex.QueryVariantVector, error) {
	if len(variants) == 0 || len(variants) != len(dense) {
		return nil, errors.New("query variants and dense vectors must have equal non-zero lengths")
	}
	result := make([]contextindex.QueryVariantVector, len(variants))
	for i, variant := range variants {
		result[i] = contextindex.QueryVariantVector{
			VariantID: variant.VariantID, QuerySHA256: variant.QuerySHA256,
			Dense:  slices.Clone(dense[i]),
			Sparse: contextindex.SparseVector{Indices: slices.Clone(variant.Sparse.Indices), Values: slices.Clone(variant.Sparse.Values)},
		}
		if err := result[i].Validate(); err != nil {
			return nil, fmt.Errorf("query vector %d: %w", i, err)
		}
	}
	return result, nil
}

func CandidatesFromQueryBatch(result contextindex.QueryBatchResult, denseMinScore *FixedScore) ([]Candidate, error) {
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCodeIndexInconsistent, err)
	}
	byPoint := make(map[[16]byte][]RetrievalBranchV1, len(result.Fusion))
	for _, hit := range result.Branches {
		score, err := FixedScoreFromFloat64(hit.Score)
		if err != nil {
			return nil, fmt.Errorf("%s: branch score: %w", ErrCodeIndexInconsistent, err)
		}
		if hit.Modality == contextindex.QueryModalityDense && denseMinScore != nil {
			comparison, compareErr := score.Compare(*denseMinScore)
			if compareErr != nil {
				return nil, compareErr
			}
			if comparison < 0 {
				continue
			}
		}
		byPoint[hit.Point.ID] = append(byPoint[hit.Point.ID], RetrievalBranchV1{
			VariantID: hit.VariantID, Modality: string(hit.Modality), Rank: hit.Rank, Score: score,
		})
	}
	candidates := make([]Candidate, 0, len(result.Fusion))
	for _, hit := range result.Fusion {
		branches := byPoint[hit.Point.ID]
		if len(branches) == 0 {
			return nil, fmt.Errorf("%s: fusion point %s has no active branch evidence", ErrCodeIndexInconsistent, hit.Point.ID)
		}
		fusion, err := FixedScoreFromFloat64(hit.Score)
		if err != nil {
			return nil, fmt.Errorf("%s: fusion score: %w", ErrCodeIndexInconsistent, err)
		}
		evidence := RetrievalBranchesV1{Schema: RetrievalBranchesSchemaV1, Branches: branches}
		if err := evidence.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", ErrCodeIndexInconsistent, err)
		}
		candidates = append(candidates, Candidate{Point: hit.Point, FusionScore: fusion, Branches: evidence})
	}
	return candidates, nil
}

func normalizeQueryText(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", errors.New("query text must be valid UTF-8")
	}
	return strings.TrimSpace(norm.NFKC.String(text)), nil
}
