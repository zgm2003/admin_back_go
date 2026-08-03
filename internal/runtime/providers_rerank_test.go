package runtime

import (
	"context"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	aiprovider "admin_back_go/internal/module/ai/provider"
)

func TestAIRerankFactoryRequiresRerankModelKind(t *testing.T) {
	factory := aiRerankFactory{}
	valid := infraai.RerankClientConfig{EngineType: infraai.EngineTypeOpenAI, ModelKind: string(aiprovider.ModelKindRerank), ModelID: "rerank-v1", BaseURL: "https://example.test/v1", Capabilities: infraai.RerankCapabilities{MaxDocuments: 8, MaxInputTokens: 4096, TokenCounterID: infraai.TokenCounterUTF8BytesV1}}
	if _, err := factory.NewRerankClient(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	valid.ModelKind = string(aiprovider.ModelKindChat)
	if _, err := factory.NewRerankClient(context.Background(), valid); !errors.Is(err, infraai.ErrRerankFailed) {
		t.Fatalf("error=%v", err)
	}
}
