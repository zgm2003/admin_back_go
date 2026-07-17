package runtime

import (
	"context"

	aichat "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/replycommand"
)

type replyAssistantRepository interface {
	PublishAssistant(context.Context, replycommand.PublishAssistantInput) (int64, bool, error)
}

type replyAssistantPublisher struct {
	repository replyAssistantRepository
}

func (p replyAssistantPublisher) PublishAssistant(ctx context.Context, input aichat.AssistantPublication) (int64, bool, error) {
	return p.repository.PublishAssistant(ctx, replycommand.PublishAssistantInput{
		CommandID: input.CommandID,
		Owner:     input.Owner,
		Token:     input.Token,
		Content:   input.Content,
		Now:       input.Now,
	})
}
