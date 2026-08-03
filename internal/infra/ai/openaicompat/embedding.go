package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int64 `json:"prompt_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

type EmbeddingClient struct {
	client       *Client
	modelID      string
	capabilities infraai.EmbeddingCapabilities
}

func NewEmbeddingClient(config Config, modelID string, capabilities infraai.EmbeddingCapabilities) (*EmbeddingClient, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("%w: embedding model ID is required", infraai.ErrEmbeddingFailed)
	}
	if err := capabilities.Validate(); err != nil {
		return nil, err
	}
	return &EmbeddingClient{client: New(config), modelID: modelID, capabilities: capabilities}, nil
}

func (bound *EmbeddingClient) Embed(ctx context.Context, texts []string) (infraai.EmbeddingResult, error) {
	if bound == nil || bound.client == nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding client is nil", infraai.ErrEmbeddingFailed)
	}
	input := infraai.EmbeddingInput{ModelID: bound.modelID, Texts: texts, Capabilities: bound.capabilities}
	return bound.client.embed(ctx, input)
}

func (client *Client) embed(ctx context.Context, input infraai.EmbeddingInput) (infraai.EmbeddingResult, error) {
	if client == nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI client is nil", infraai.ErrEmbeddingFailed)
	}
	if err := input.Validate(); err != nil {
		return infraai.EmbeddingResult{}, err
	}
	request, err := client.newRequest(ctx, http.MethodPost, "/embeddings", embeddingRequest{Model: input.ModelID, Input: input.Texts})
	if err != nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: build OpenAI embedding request: %v", infraai.ErrEmbeddingFailed, err)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding request failed", infraai.ErrEmbeddingFailed)
	}
	defer response.Body.Close()
	if err := client.requireSuccess(ctx, response); err != nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding request rejected", infraai.ErrEmbeddingFailed)
	}
	var decoded embeddingResponse
	maxResponseBytes := int64(input.Capabilities.MaxInputs)*int64(input.Capabilities.Dimensions)*16 + 64<<10
	limited := &io.LimitedReader{R: response.Body, N: maxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(&decoded); err != nil {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: decode OpenAI embedding response", infraai.ErrEmbeddingFailed)
	}
	if limited.N <= 0 {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding response exceeds declared shape", infraai.ErrEmbeddingFailed)
	}
	if len(decoded.Data) != len(input.Texts) {
		return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding response count disagrees", infraai.ErrEmbeddingFailed)
	}
	sort.Slice(decoded.Data, func(i, j int) bool { return decoded.Data[i].Index < decoded.Data[j].Index })
	vectors := make([][]float32, len(decoded.Data))
	for position, item := range decoded.Data {
		if item.Index != position {
			return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding indices are not complete", infraai.ErrEmbeddingFailed)
		}
		vectors[position] = make([]float32, len(item.Embedding))
		for index, value := range item.Embedding {
			converted := float32(value)
			if math.IsNaN(value) || math.IsInf(value, 0) || math.IsInf(float64(converted), 0) {
				return infraai.EmbeddingResult{}, fmt.Errorf("%w: OpenAI embedding contains a non-finite value", infraai.ErrEmbeddingFailed)
			}
			vectors[position][index] = converted
		}
	}
	result := infraai.EmbeddingResult{
		ModelID: decoded.Model, Vectors: vectors,
		Usage: infraai.EmbeddingUsage{PromptTokens: decoded.Usage.PromptTokens, TotalTokens: decoded.Usage.TotalTokens},
	}
	if err := result.Validate(input); err != nil {
		return infraai.EmbeddingResult{}, err
	}
	return result, nil
}
