package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type replyCommandPollRunner interface {
	RunOnce(context.Context) (bool, error)
}

func startReplyCommandPoller(parent context.Context, runner replyCommandPollRunner, interval time.Duration, concurrency int, logger *slog.Logger) (CleanupFunc, error) {
	if runner == nil {
		return nil, errors.New("reply command poll runner is required")
	}
	if interval <= 0 {
		return nil, errors.New("reply command poll interval must be positive")
	}
	if concurrency <= 0 {
		return nil, errors.New("reply command poll concurrency must be positive")
	}
	if parent == nil {
		parent = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(parent)
	semaphore := make(chan struct{}, concurrency)
	var workers sync.WaitGroup
	done := make(chan struct{})
	run := func() {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return
		default:
			return
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			worked, err := runner.RunOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.ErrorContext(ctx, "reply command polling failed", "worked", worked, "error", err)
			}
		}()
	}
	go func() {
		defer close(done)
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				workers.Wait()
				return
			case <-ticker.C:
				run()
			}
		}
	}()

	var once sync.Once
	return func(shutdownCtx context.Context) error {
		once.Do(cancel)
		if shutdownCtx == nil {
			shutdownCtx = context.Background()
		}
		select {
		case <-done:
			return nil
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}, nil
}
