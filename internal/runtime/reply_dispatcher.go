package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	aichat "admin_back_go/internal/module/ai/chat"
	aimessage "admin_back_go/internal/module/ai/message"
)

const aiConversationReplyTimeout = 2 * time.Minute

type aiConversationReplyDispatcher struct {
	service conversationReplyExecutor
	logger  *slog.Logger
	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
	runs   map[string]*replyRun
}

type replyRun struct {
	cancel context.CancelFunc
}

type conversationReplyExecutor interface {
	ExecuteConversationReply(context.Context, aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error)
}

func newAIConversationReplyDispatcher(service conversationReplyExecutor, logger *slog.Logger, timeout time.Duration) *aiConversationReplyDispatcher {
	if timeout <= 0 {
		timeout = aiConversationReplyTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &aiConversationReplyDispatcher{
		service: service,
		logger:  logger,
		timeout: timeout,
		ctx:     ctx,
		cancel:  cancel,
		runs:    map[string]*replyRun{},
	}
}

func (dispatcher *aiConversationReplyDispatcher) EnqueueConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	if dispatcher == nil || dispatcher.service == nil {
		return errors.New("ai conversation reply service is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dispatcher.mu.Lock()
	if dispatcher.closed {
		dispatcher.mu.Unlock()
		return errors.New("ai conversation reply dispatcher is closed")
	}
	key := replyKey(payload.ConversationID, payload.RequestID)
	if oldRun := dispatcher.runs[key]; oldRun != nil {
		oldRun.cancel()
	}
	runCtx, runCancel := context.WithTimeout(dispatcher.ctx, dispatcher.timeout)
	run := &replyRun{cancel: runCancel}
	dispatcher.runs[key] = run
	dispatcher.wg.Add(1)
	dispatcher.mu.Unlock()

	input := aichat.ConversationReplyInput{
		ConversationID: payload.ConversationID,
		UserID:         payload.UserID,
		AgentID:        payload.AgentID,
		UserMessageID:  payload.UserMessageID,
		RequestID:      payload.RequestID,
	}
	go func() {
		defer func() {
			runCancel()
			dispatcher.mu.Lock()
			if dispatcher.runs[key] == run {
				delete(dispatcher.runs, key)
			}
			dispatcher.mu.Unlock()
			dispatcher.wg.Done()
		}()
		if _, err := dispatcher.service.ExecuteConversationReply(runCtx, input); err != nil {
			dispatcher.logger.ErrorContext(runCtx, "ai conversation reply failed", "conversation_id", input.ConversationID, "request_id", input.RequestID, "error", err)
		}
	}()
	return nil
}

func (dispatcher *aiConversationReplyDispatcher) CancelConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	if dispatcher == nil {
		return errors.New("ai conversation reply dispatcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := replyKey(payload.ConversationID, payload.RequestID)
	if key == "" {
		return errors.New("ai conversation reply cancel key is invalid")
	}
	dispatcher.mu.Lock()
	run := dispatcher.runs[key]
	dispatcher.mu.Unlock()
	if run != nil {
		run.cancel()
	}
	return nil
}

func (dispatcher *aiConversationReplyDispatcher) Shutdown(ctx context.Context) error {
	if dispatcher == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dispatcher.mu.Lock()
	if dispatcher.closed {
		dispatcher.mu.Unlock()
		return nil
	}
	dispatcher.closed = true
	for _, run := range dispatcher.runs {
		run.cancel()
	}
	dispatcher.runs = map[string]*replyRun{}
	dispatcher.cancel()
	dispatcher.mu.Unlock()

	done := make(chan struct{})
	go func() {
		dispatcher.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func replyKey(conversationID int64, requestID string) string {
	if conversationID <= 0 || requestID == "" {
		return ""
	}
	return fmt.Sprintf("%d:%s", conversationID, requestID)
}
