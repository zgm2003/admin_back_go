package runtime

import (
	"context"
	"errors"
	"math"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/contextengine"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/enum"
)

type memoryProviderSummarizer struct {
	db      *database.Client
	factory aichat.EngineFactory
	box     secretbox.Box
}

func newMemoryProviderSummarizer(db *database.Client, factory aichat.EngineFactory, box secretbox.Box) contextengine.MemorySummarizer {
	if db == nil || db.Gorm == nil || factory == nil {
		return nil
	}
	return &memoryProviderSummarizer{db: db, factory: factory, box: box}
}

func (summarizer *memoryProviderSummarizer) Summarize(ctx context.Context, request contextengine.MemorySummaryRequest) (contextengine.MemorySummaryResult, error) {
	if summarizer == nil || summarizer.db == nil || summarizer.db.Gorm == nil || summarizer.factory == nil ||
		request.ProviderModelID == 0 || strings.TrimSpace(request.Prompt) == "" || request.MaxOutputTokens == 0 || request.MaxOutputTokens > math.MaxInt {
		return contextengine.MemorySummaryResult{}, errors.New("memory provider request is invalid")
	}
	var row struct {
		ModelID         string `gorm:"column:model_id"`
		ModelKind       string `gorm:"column:model_kind"`
		ModelStatus     int    `gorm:"column:model_status"`
		EngineType      string `gorm:"column:engine_type"`
		APIProtocol     string `gorm:"column:api_protocol"`
		BaseURL         string `gorm:"column:base_url"`
		APIKeyEnc       string `gorm:"column:api_key_enc"`
		ProviderStatus  int    `gorm:"column:provider_status"`
		ProviderDeleted int    `gorm:"column:provider_deleted"`
	}
	err := summarizer.db.Gorm.WithContext(ctx).Table("ai_provider_models AS pm").
		Select("pm.model_id, pm.model_kind, pm.status AS model_status, p.engine_type, p.api_protocol, p.base_url, p.api_key_enc, p.status AS provider_status, p.is_del AS provider_deleted").
		Joins("JOIN ai_providers AS p ON p.id = pm.provider_id").Where("pm.id = ?", request.ProviderModelID).Take(&row).Error
	if err != nil {
		return contextengine.MemorySummaryResult{}, err
	}
	if row.ModelKind != string(aiprovider.ModelKindChat) || row.ModelStatus != enum.CommonYes || row.ProviderStatus != enum.CommonYes || row.ProviderDeleted != enum.CommonNo {
		return contextengine.MemorySummaryResult{}, errors.New("memory provider model is not enabled")
	}
	apiKey, err := summarizer.box.Decrypt(row.APIKeyEnc)
	if err != nil {
		return contextengine.MemorySummaryResult{}, err
	}
	engine, err := summarizer.factory.NewEngine(ctx, aichat.EngineConfig{EngineType: infraai.EngineType(row.EngineType), BaseURL: row.BaseURL, APIKey: apiKey, APIProtocol: row.APIProtocol})
	if err != nil {
		return contextengine.MemorySummaryResult{}, err
	}
	result, err := engine.StreamChat(ctx, infraai.ChatInput{ModelID: row.ModelID, EffectiveMaxOutputTokens: int(request.MaxOutputTokens), Messages: []infraai.Message{
		{Role: infraai.MessageRoleSystem, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: "Summarize conversation history exactly as requested. Return only the memory."}}},
		{Role: infraai.MessageRoleUser, Parts: []infraai.ContentPart{{Kind: infraai.ContentPartText, Text: request.Prompt}}},
	}}, nil)
	if err != nil {
		return contextengine.MemorySummaryResult{}, err
	}
	if result == nil || strings.TrimSpace(result.Answer) == "" || result.PromptTokens < 0 || result.CompletionTokens < 0 {
		return contextengine.MemorySummaryResult{}, errors.New("memory provider returned an invalid result")
	}
	return contextengine.MemorySummaryResult{Summary: result.Answer, PromptTokens: uint64(result.PromptTokens),
		CompletionTokens: uint64(result.CompletionTokens), ProviderRequestID: result.ProviderRequestID}, nil
}

var _ contextengine.MemorySummarizer = (*memoryProviderSummarizer)(nil)
