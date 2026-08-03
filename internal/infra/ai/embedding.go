package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrEmbeddingFailed = errors.New("ai.context.embedding_failed")

type EmbeddingCapabilities struct {
	Dimensions     uint32
	MaxInputs      uint32
	MaxInputTokens int64
	TokenCounterID string
}

func (capabilities EmbeddingCapabilities) Validate() error {
	if capabilities.Dimensions == 0 || capabilities.MaxInputs == 0 || capabilities.MaxInputTokens <= 0 {
		return fmt.Errorf("%w: explicit embedding dimensions and input limits are required", ErrEmbeddingFailed)
	}
	if _, err := ResolveTokenCounter(capabilities.TokenCounterID); err != nil {
		return fmt.Errorf("%w: token counter is not registered", ErrEmbeddingFailed)
	}
	return nil
}

type EmbeddingInput struct {
	ModelID      string
	Texts        []string
	Capabilities EmbeddingCapabilities
}

func (input EmbeddingInput) Validate() error {
	if strings.TrimSpace(input.ModelID) == "" || strings.TrimSpace(input.ModelID) != input.ModelID || len(input.Texts) == 0 {
		return fmt.Errorf("%w: model and inputs are required", ErrEmbeddingFailed)
	}
	if err := input.Capabilities.Validate(); err != nil {
		return err
	}
	if len(input.Texts) > int(input.Capabilities.MaxInputs) {
		return fmt.Errorf("%w: embedding input count exceeds capability", ErrEmbeddingFailed)
	}
	counter, _ := ResolveTokenCounter(input.Capabilities.TokenCounterID)
	for _, text := range input.Texts {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("%w: embedding text is empty", ErrEmbeddingFailed)
		}
		bound, err := counter.UpperBoundText(text)
		if err != nil || bound > input.Capabilities.MaxInputTokens {
			return fmt.Errorf("%w: embedding input exceeds token capability", ErrEmbeddingFailed)
		}
	}
	return nil
}

type EmbeddingUsage struct {
	PromptTokens int64
	TotalTokens  int64
}

type EmbeddingResult struct {
	ModelID string
	Vectors [][]float32
	Usage   EmbeddingUsage
}

func (result EmbeddingResult) Validate(input EmbeddingInput) error {
	if result.ModelID != input.ModelID || len(result.Vectors) != len(input.Texts) || result.Usage.PromptTokens < 0 || result.Usage.TotalTokens < result.Usage.PromptTokens {
		return fmt.Errorf("%w: provider embedding identity, count, or usage disagrees", ErrEmbeddingFailed)
	}
	for _, vector := range result.Vectors {
		if len(vector) != int(input.Capabilities.Dimensions) {
			return fmt.Errorf("%w: provider embedding dimensions disagree", ErrEmbeddingFailed)
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fmt.Errorf("%w: provider embedding contains a non-finite value", ErrEmbeddingFailed)
			}
		}
	}
	return nil
}

type EmbeddingClient interface {
	Embed(context.Context, EmbeddingInput) (EmbeddingResult, error)
}

type EmbeddingClientConfig struct {
	EngineType   EngineType
	ModelKind    string
	BaseURL      string
	APIKey       string
	Capabilities EmbeddingCapabilities
}

type EmbeddingFactory interface {
	NewEmbeddingClient(context.Context, EmbeddingClientConfig) (EmbeddingClient, error)
}
