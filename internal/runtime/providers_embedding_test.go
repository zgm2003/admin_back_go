package runtime

import (
	"context"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	aiprovider "admin_back_go/internal/module/ai/provider"
)

func TestEmbeddingFactoryRequiresEmbeddingKindAndSupportedEngine(t *testing.T) {
	factory := aiEmbeddingFactory{}
	valid := infraai.EmbeddingClientConfig{
		EngineType: infraai.EngineTypeOpenAI, ModelKind: string(aiprovider.ModelKindEmbedding),
		BaseURL: "https://api.example.com/v1", APIKey: "secret", ModelID: "embed-v1",
		Capabilities: infraai.EmbeddingCapabilities{Dimensions: 3, MaxInputs: 2, MaxInputTokens: 20, TokenCounterID: infraai.TokenCounterUTF8BytesV1},
	}
	if _, err := factory.NewEmbeddingClient(context.Background(), valid); err != nil {
		t.Fatalf("valid embedding config: %v", err)
	}
	invalidKind := valid
	invalidKind.ModelKind = string(aiprovider.ModelKindChat)
	if _, err := factory.NewEmbeddingClient(context.Background(), invalidKind); !errors.Is(err, infraai.ErrEmbeddingFailed) {
		t.Fatalf("chat kind error=%v want ErrEmbeddingFailed", err)
	}
	invalidEngine := valid
	invalidEngine.EngineType = infraai.EngineType("unsupported")
	if _, err := factory.NewEmbeddingClient(context.Background(), invalidEngine); !errors.Is(err, infraai.ErrEmbeddingFailed) {
		t.Fatalf("unsupported engine error=%v want ErrEmbeddingFailed", err)
	}
}
