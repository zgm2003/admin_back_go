package contextengine

import (
	"context"
	"crypto/sha256"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/contextindex"
)

type staleConversationIndexRepository struct{}

func (staleConversationIndexRepository) LoadConversationIndexWork(context.Context, ContextConversationIndexV1) (ConversationIndexWork, error) {
	return ConversationIndexWork{}, ErrConversationTurnNotAuthoritative
}
func (staleConversationIndexRepository) BuildConversationIndexPayload(context.Context, uint64, uint64) (ContextConversationIndexV1, error) {
	return ContextConversationIndexV1{}, ErrConversationTurnNotAuthoritative
}

type unusedConversationEmbeddingResolver struct{}

func (unusedConversationEmbeddingResolver) ResolveEmbedding(context.Context, ContextProfile) (infraai.EmbeddingClient, error) {
	return nil, nil
}

func TestConversationIndexDropsStaleTurnWithoutEmbeddingOrRetry(t *testing.T) {
	service := NewConversationIndexService(ConversationIndexDependencies{
		Repository: staleConversationIndexRepository{}, Embeddings: unusedConversationEmbeddingResolver{}, Index: &consistencyIndexStub{}, CollectionPrefix: "ctx",
	})
	if err := service.IndexConversationTurn(t.Context(), ContextConversationIndexV1{ProfileID: 7, ConversationID: 11, UserMessageID: 13, SourceSHA256: sha256.Sum256([]byte("old"))}); err != nil {
		t.Fatal(err)
	}
}

func TestConversationIndexTaskIdentityIncludesClosedTurnFacts(t *testing.T) {
	payload := ContextConversationIndexV1{
		ProfileID: 7, ConversationID: 11, UserMessageID: 13,
		SourceSHA256: sha256.Sum256([]byte("turn")),
	}
	first, err := ConversationIndexIdempotencyKey(payload)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := ConversationIndexIdempotencyKey(payload); err != nil || again != first {
		t.Fatalf("idempotency key changed: key=%q err=%v", again, err)
	}
	payload.SourceSHA256 = sha256.Sum256([]byte("changed"))
	if changed, err := ConversationIndexIdempotencyKey(payload); err != nil || changed == first {
		t.Fatalf("changed Turn facts reused key: key=%q err=%v", changed, err)
	}
}

func TestConversationTurnPointIsPrivateAndDeterministic(t *testing.T) {
	turn := ConversationTurn{
		ConversationID: 11, UserID: 5, AgentID: 3,
		UserMessage:       TurnMessage{ID: 13, Role: "user", Content: "question"},
		AssistantMessage:  TurnMessage{ID: 14, Role: "assistant", Content: "answer"},
		AssistantDelivery: "completed",
	}
	if err := turn.ComputeSourceSHA256(); err != nil {
		t.Fatal(err)
	}
	work := ConversationIndexWork{Profile: ContextProfile{ID: 7}, Platform: "admin", IndexGeneration: 2, Turn: turn}
	point, err := conversationTurnPoint(work, []float32{1, 2}, contextindex.SparseVector{})
	if err != nil {
		t.Fatal(err)
	}
	metadata := point.Metadata
	if metadata.ScopeKind != contextindex.ScopeKindConversation || metadata.ConversationID != 11 || metadata.UserID != 5 ||
		metadata.Ref.SourceKind != contextindex.SourceKindConversationTurn || metadata.Ref.SourceID != 13 || metadata.Ref.SourceSHA256 != turn.SourceSHA256 {
		t.Fatalf("point metadata=%+v", metadata)
	}
	again, err := conversationTurnPoint(work, []float32{1, 2}, contextindex.SparseVector{})
	if err != nil || again.Metadata.Ref.ID != metadata.Ref.ID {
		t.Fatalf("point identity changed: point=%+v err=%v", again.Metadata.Ref, err)
	}
}
