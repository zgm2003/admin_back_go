package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrCleanupNameRequired = errors.New("cleanup name is required")
	ErrCleanupFuncRequired = errors.New("cleanup function is required")
	ErrCleanupClosed       = errors.New("cleanup is already closed")
)

type CleanupFunc func(context.Context) error

type cleanupEntry struct {
	name string
	fn   CleanupFunc
}

type Cleanup struct {
	mu      sync.Mutex
	closed  bool
	entries []cleanupEntry
}

func NewCleanup() *Cleanup {
	return &Cleanup{}
}

func (c *Cleanup) Add(name string, fn CleanupFunc) error {
	if strings.TrimSpace(name) == "" {
		return ErrCleanupNameRequired
	}
	if fn == nil {
		return ErrCleanupFuncRequired
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrCleanupClosed
	}
	c.entries = append(c.entries, cleanupEntry{name: strings.TrimSpace(name), fn: fn})
	return nil
}

func (c *Cleanup) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	entries := append([]cleanupEntry(nil), c.entries...)
	c.entries = nil
	c.mu.Unlock()

	failures := make([]error, 0, len(entries))
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if err := entry.fn(ctx); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", entry.name, err))
		}
	}
	return errors.Join(failures...)
}
