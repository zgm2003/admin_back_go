package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

var ErrRerankFailed = errors.New("ai.context.rerank_failed")

type RerankCapabilities struct {
	MaxDocuments   uint32
	MaxInputTokens int64
	TokenCounterID string
}

func (capabilities RerankCapabilities) Validate() error {
	if capabilities.MaxDocuments == 0 || capabilities.MaxInputTokens <= 0 {
		return fmt.Errorf("%w: explicit rerank limits are required", ErrRerankFailed)
	}
	if _, err := ResolveTokenCounter(capabilities.TokenCounterID); err != nil {
		return fmt.Errorf("%w: token counter is not registered", ErrRerankFailed)
	}
	return nil
}

type RerankDocument struct {
	CandidateID string
	Text        string
}

type RerankInput struct {
	ModelID      string
	Query        string
	Documents    []RerankDocument
	Capabilities RerankCapabilities
}

func (input RerankInput) Validate() error {
	if strings.TrimSpace(input.ModelID) == "" || strings.TrimSpace(input.ModelID) != input.ModelID ||
		strings.TrimSpace(input.Query) == "" || !utf8.ValidString(input.Query) || len(input.Documents) == 0 {
		return fmt.Errorf("%w: model, query and documents are required", ErrRerankFailed)
	}
	if err := input.Capabilities.Validate(); err != nil {
		return err
	}
	if len(input.Documents) > int(input.Capabilities.MaxDocuments) {
		return fmt.Errorf("%w: document count exceeds capability", ErrRerankFailed)
	}
	counter, _ := ResolveTokenCounter(input.Capabilities.TokenCounterID)
	total, err := counter.UpperBoundText(input.Query)
	if err != nil {
		return fmt.Errorf("%w: count query tokens", ErrRerankFailed)
	}
	seen := make(map[string]struct{}, len(input.Documents))
	for _, document := range input.Documents {
		if strings.TrimSpace(document.CandidateID) == "" || strings.TrimSpace(document.CandidateID) != document.CandidateID ||
			strings.TrimSpace(document.Text) == "" || !utf8.ValidString(document.Text) {
			return fmt.Errorf("%w: candidate identity and text are required", ErrRerankFailed)
		}
		if _, duplicate := seen[document.CandidateID]; duplicate {
			return fmt.Errorf("%w: duplicate candidate identity", ErrRerankFailed)
		}
		seen[document.CandidateID] = struct{}{}
		bound, countErr := counter.UpperBoundText(document.Text)
		if countErr != nil {
			return fmt.Errorf("%w: count document tokens", ErrRerankFailed)
		}
		total += bound
		if total > input.Capabilities.MaxInputTokens {
			return fmt.Errorf("%w: single request token limit exceeded", ErrRerankFailed)
		}
	}
	return nil
}

type RerankScore struct {
	CandidateID string
	Score       float64
}

type RerankUsage struct {
	TotalTokens int64
}

type RerankResult struct {
	ModelID string
	Scores  []RerankScore
	Usage   RerankUsage
}

func (result RerankResult) Validate(input RerankInput) error {
	if result.ModelID != input.ModelID || len(result.Scores) != len(input.Documents) || result.Usage.TotalTokens < 0 {
		return fmt.Errorf("%w: provider rerank identity, count or usage disagrees", ErrRerankFailed)
	}
	expected := make(map[string]struct{}, len(input.Documents))
	for _, document := range input.Documents {
		expected[document.CandidateID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(result.Scores))
	for _, score := range result.Scores {
		if _, ok := expected[score.CandidateID]; !ok || math.IsNaN(score.Score) || math.IsInf(score.Score, 0) || score.Score < 0 || score.Score > 1 {
			return fmt.Errorf("%w: provider rerank candidate or score disagrees", ErrRerankFailed)
		}
		if _, duplicate := seen[score.CandidateID]; duplicate {
			return fmt.Errorf("%w: provider rerank candidate is duplicated", ErrRerankFailed)
		}
		seen[score.CandidateID] = struct{}{}
	}
	return nil
}

type RerankClient interface {
	Rerank(context.Context, string, []RerankDocument) (RerankResult, error)
}

type RerankClientConfig struct {
	EngineType   EngineType
	ModelKind    string
	ModelID      string
	BaseURL      string
	APIKey       string
	Capabilities RerankCapabilities
}

type RerankFactory interface {
	NewRerankClient(context.Context, RerankClientConfig) (RerankClient, error)
}
