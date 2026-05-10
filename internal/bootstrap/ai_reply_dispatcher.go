package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"admin_back_go/internal/module/aichat"
	"admin_back_go/internal/module/aimessage"
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
	ExecuteConversationReply(ctx context.Context, input aichat.ConversationReplyInput) (*aichat.ConversationReplyResult, error)
}

func newAIConversationReplyDispatcher(service conversationReplyExecutor, logger *slog.Logger, timeout time.Duration) *aiConversationReplyDispatcher {
	if timeout <= 0 {
		timeout = aiConversationReplyTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &aiConversationReplyDispatcher{service: service, logger: logger, timeout: timeout, ctx: ctx, cancel: cancel, runs: map[string]*replyRun{}}
}

func (d *aiConversationReplyDispatcher) EnqueueConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	if d == nil || d.service == nil {
		return errors.New("ai conversation reply service is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return errors.New("ai conversation reply dispatcher is closed")
	}
	key := replyKey(payload.ConversationID, payload.RequestID)
	if oldRun := d.runs[key]; oldRun != nil {
		oldRun.cancel()
	}
	runCtx, runCancel := context.WithTimeout(d.ctx, d.timeout)
	run := &replyRun{cancel: runCancel}
	d.runs[key] = run
	d.wg.Add(1)
	d.mu.Unlock()

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
			d.mu.Lock()
			if d.runs[key] == run {
				delete(d.runs, key)
			}
			d.mu.Unlock()
			d.wg.Done()
		}()
		if _, err := d.service.ExecuteConversationReply(runCtx, input); err != nil {
			d.logger.ErrorContext(runCtx, "ai conversation reply failed", "conversation_id", input.ConversationID, "request_id", input.RequestID, "error", err)
		}
	}()
	return nil
}

func (d *aiConversationReplyDispatcher) CancelConversationReply(ctx context.Context, payload aimessage.ReplyPayload) error {
	if d == nil {
		return errors.New("ai conversation reply dispatcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := replyKey(payload.ConversationID, payload.RequestID)
	if key == "" {
		return errors.New("ai conversation reply cancel key is invalid")
	}
	d.mu.Lock()
	run := d.runs[key]
	d.mu.Unlock()
	if run != nil {
		run.cancel()
	}
	return nil
}

func (d *aiConversationReplyDispatcher) Shutdown(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	for _, run := range d.runs {
		run.cancel()
	}
	d.runs = map[string]*replyRun{}
	d.cancel()
	d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
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
