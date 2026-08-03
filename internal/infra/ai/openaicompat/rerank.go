package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

type rerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

type rerankResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"usage"`
}

type RerankClient struct {
	client       *Client
	modelID      string
	capabilities infraai.RerankCapabilities
}

func NewRerankClient(config Config, modelID string, capabilities infraai.RerankCapabilities) (*RerankClient, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("%w: rerank model ID is required", infraai.ErrRerankFailed)
	}
	if err := capabilities.Validate(); err != nil {
		return nil, err
	}
	return &RerankClient{client: New(config), modelID: modelID, capabilities: capabilities}, nil
}

func (bound *RerankClient) Rerank(ctx context.Context, query string, documents []infraai.RerankDocument) (infraai.RerankResult, error) {
	if bound == nil || bound.client == nil {
		return infraai.RerankResult{}, fmt.Errorf("%w: OpenAI rerank client is nil", infraai.ErrRerankFailed)
	}
	input := infraai.RerankInput{ModelID: bound.modelID, Query: query, Documents: documents, Capabilities: bound.capabilities}
	if err := input.Validate(); err != nil {
		return infraai.RerankResult{}, err
	}
	texts := make([]string, len(documents))
	for i, document := range documents {
		texts[i] = document.Text
	}
	request, err := bound.client.newRequest(ctx, http.MethodPost, "/rerank", rerankRequest{
		Model: input.ModelID, Query: input.Query, Documents: texts, TopN: len(texts), ReturnDocuments: false,
	})
	if err != nil {
		return infraai.RerankResult{}, fmt.Errorf("%w: build OpenAI rerank request", infraai.ErrRerankFailed)
	}
	response, err := bound.client.httpClient.Do(request)
	if err != nil {
		return infraai.RerankResult{}, fmt.Errorf("%w: OpenAI rerank request failed", infraai.ErrRerankFailed)
	}
	defer response.Body.Close()
	if err := bound.client.requireSuccess(ctx, response); err != nil {
		return infraai.RerankResult{}, fmt.Errorf("%w: OpenAI rerank request rejected", infraai.ErrRerankFailed)
	}
	maxResponseBytes := int64(len(documents))*256 + 64<<10
	limited := &io.LimitedReader{R: response.Body, N: maxResponseBytes + 1}
	var decoded rerankResponse
	if err := json.NewDecoder(limited).Decode(&decoded); err != nil || limited.N <= 0 {
		return infraai.RerankResult{}, fmt.Errorf("%w: decode OpenAI rerank response", infraai.ErrRerankFailed)
	}
	scores := make([]infraai.RerankScore, len(decoded.Results))
	seen := make(map[int]struct{}, len(decoded.Results))
	for i, item := range decoded.Results {
		if item.Index < 0 || item.Index >= len(documents) || math.IsNaN(item.RelevanceScore) || math.IsInf(item.RelevanceScore, 0) {
			return infraai.RerankResult{}, fmt.Errorf("%w: OpenAI rerank result is invalid", infraai.ErrRerankFailed)
		}
		if _, duplicate := seen[item.Index]; duplicate {
			return infraai.RerankResult{}, fmt.Errorf("%w: OpenAI rerank result is duplicated", infraai.ErrRerankFailed)
		}
		seen[item.Index] = struct{}{}
		scores[i] = infraai.RerankScore{CandidateID: documents[item.Index].CandidateID, Score: item.RelevanceScore}
	}
	result := infraai.RerankResult{ModelID: decoded.Model, Scores: scores, Usage: infraai.RerankUsage{TotalTokens: decoded.Usage.TotalTokens}}
	if err := result.Validate(input); err != nil {
		return infraai.RerankResult{}, err
	}
	return result, nil
}
