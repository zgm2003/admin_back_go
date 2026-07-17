package runtime

import (
	"context"
	"testing"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type fakeReplyAssistantRepository struct {
	input replycommand.PublishAssistantInput
}

func (f *fakeReplyAssistantRepository) PublishAssistant(_ context.Context, input replycommand.PublishAssistantInput) (int64, bool, error) {
	f.input = input
	return 22, true, nil
}

func TestReplyAssistantPublisherMapsFencingIdentity(t *testing.T) {
	repository := &fakeReplyAssistantRepository{}
	publisher := replyAssistantPublisher{repository: repository}
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	id, published, err := publisher.PublishAssistant(context.Background(), aichat.AssistantPublication{CommandID: 41, Owner: "worker-a", Token: 7, Content: "answer", Now: now})
	if err != nil || !published || id != 22 {
		t.Fatalf("id=%d published=%v err=%v", id, published, err)
	}
	if repository.input.CommandID != 41 || repository.input.Owner != "worker-a" || repository.input.Token != 7 || repository.input.Content != "answer" || !repository.input.Now.Equal(now) {
		t.Fatalf("mapped input=%+v", repository.input)
	}
}
