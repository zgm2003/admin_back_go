package aichat

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	conversation *Conversation
	agent        *AgentEngineConfig
	history      []MessageHistory
	assistant    AssistantMessageRecord
	timeoutLimit int
}

func (f *fakeRepository) ConversationForReply(ctx context.Context, id int64, userID int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) AgentForRuntime(ctx context.Context, agentID uint64) (*AgentEngineConfig, error) {
	return f.agent, nil
}
func (f *fakeRepository) LatestMessages(ctx context.Context, conversationID int64, limit int) ([]MessageHistory, error) {
	return f.history, nil
}
func (f *fakeRepository) InsertAssistantMessage(ctx context.Context, input AssistantMessageRecord) (int64, error) {
	f.assistant = input
	return 22, nil
}
func (f *fakeRepository) TimeoutRuns(ctx context.Context, limit int, message string) (int64, error) {
	f.timeoutLimit = limit
	return 2, nil
}

type fakePublisher struct {
	pubs []platformrealtime.Publication
}

func (f *fakePublisher) Publish(ctx context.Context, p platformrealtime.Publication) error {
	f.pubs = append(f.pubs, p)
	return nil
}

type fakeEngineFactory struct {
	engine platformai.Engine
	input  EngineConfig
	err    error
}

func (f *fakeEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

func validAgentConfig(t *testing.T) (*AgentEngineConfig, secretbox.Box) {
	t.Helper()
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("provider-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	return &AgentEngineConfig{
		AgentID:         5,
		AgentName:       "客服",
		ProviderID:      2,
		ModelID:         "gpt-5.4",
		ScenesJSON:      `["chat"]`,
		EngineType:      string(platformai.EngineTypeDify),
		EngineBaseURL:   "https://dify.test/v1",
		EngineAPIKeyEnc: cipher,
		AgentStatus:     enum.CommonYes,
		EngineStatus:    enum.CommonYes,
	}, box
}

func TestExecuteConversationReplyPublishesConversationScopedEventsAndPersistsAssistant(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5, IsDel: enum.CommonNo},
		agent:        agent,
		history: []MessageHistory{
			{ID: 9, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "hi"},
		},
	}
	pub := &fakePublisher{}
	factory := &fakeEngineFactory{engine: platformai.NewFakeEngine("ok")}
	res, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: factory, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err != nil {
		t.Fatalf("ExecuteConversationReply returned error: %v", err)
	}
	if res.AssistantMessageID != 22 || repo.assistant.Content != "ok" || repo.assistant.ConversationID != 3 {
		t.Fatalf("unexpected assistant result: res=%#v assistant=%#v", res, repo.assistant)
	}
	if factory.input.APIKey != "provider-key" || factory.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected engine config: %#v", factory.input)
	}
	if len(pub.pubs) < 3 || pub.pubs[0].Envelope.Type != EventAIResponseStart || pub.pubs[1].Envelope.Type != EventAIResponseDelta || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseCompleted {
		t.Fatalf("unexpected publications: %#v", pub.pubs)
	}
	for _, pub := range pub.pubs {
		if pub.UserID != 7 || pub.Platform != enum.PlatformAdmin {
			t.Fatalf("publication not scoped to current admin user: %#v", pub)
		}
	}
}

func TestExecuteConversationReplyPublishesFailedForEngineError(t *testing.T) {
	agent, box := validAgentConfig(t)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent, history: []MessageHistory{{ID: 9, Role: enum.AIMessageRoleUser, Content: "hi"}}}
	pub := &fakePublisher{}
	_, err := NewService(Dependencies{Repository: repo, Publisher: pub, EngineFactory: &fakeEngineFactory{engine: &platformai.FakeEngine{Err: errors.New("engine down")}}, Secretbox: box}).ExecuteConversationReply(context.Background(), ConversationReplyInput{ConversationID: 3, UserID: 7, AgentID: 5, UserMessageID: 9, RequestID: "rid"})
	if err == nil {
		t.Fatal("expected engine error")
	}
	if len(pub.pubs) == 0 || pub.pubs[len(pub.pubs)-1].Envelope.Type != EventAIResponseFailed {
		t.Fatalf("expected failed publication, got %#v", pub.pubs)
	}
}

func TestTimeoutRunsMarksOldRunsFailed(t *testing.T) {
	repo := &fakeRepository{}
	res, err := NewService(Dependencies{Repository: repo}).TimeoutRuns(context.Background(), RunTimeoutInput{Limit: 5})
	if err != nil {
		t.Fatalf("TimeoutRuns returned error: %v", err)
	}
	if repo.timeoutLimit != 5 || res.Failed != 2 {
		t.Fatalf("unexpected timeout result: repo=%#v res=%#v", repo, res)
	}
}

func TestChatHistoryExcludesCurrentUserMessageAndKeepsOrder(t *testing.T) {
	now := time.Now()
	history := chatHistory([]MessageHistory{
		{ID: 3, Role: enum.AIMessageRoleAssistant, Content: "two", CreatedAt: now},
		{ID: 1, Role: enum.AIMessageRoleUser, Content: "one", CreatedAt: now},
		{ID: 4, Role: enum.AIMessageRoleUser, Content: "current", CreatedAt: now},
	}, 4)
	if len(history) != 2 || history[0]["content"] != "one" || history[1]["content"] != "two" {
		t.Fatalf("unexpected history: %#v", history)
	}
}
