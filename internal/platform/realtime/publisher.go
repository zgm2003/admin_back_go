package realtime

import (
	"context"
	"errors"
	"strings"
)

var ErrPublicationTargetRequired = errors.New("realtime publication target required")

// Publication is one server-side realtime message delivery request. It is an
// internal contract; business modules publish envelopes without knowing whether
// delivery is local-only or Redis-backed later.
type Publication struct {
	SessionKey string
	Envelope   Envelope
}

// Publisher sends realtime envelopes to connected clients. Implementations may
// be local-process, Redis Pub/Sub, Redis Streams, or a test/no-op publisher.
type Publisher interface {
	Publish(context.Context, Publication) error
}

// LocalPublisher publishes to the in-process Manager only. It is the first
// foundation step, not a distributed fan-out implementation.
type LocalPublisher struct {
	manager *Manager
}

// NewLocalPublisher creates a local-process realtime publisher.
func NewLocalPublisher(manager *Manager) *LocalPublisher {
	return &LocalPublisher{manager: manager}
}

// Publish sends one envelope to a local session key.
func (p *LocalPublisher) Publish(ctx context.Context, publication Publication) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := strings.TrimSpace(publication.SessionKey)
	if key == "" {
		return ErrPublicationTargetRequired
	}
	if p == nil || p.manager == nil {
		return ErrSessionNotFound
	}
	return p.manager.Send(key, publication.Envelope)
}

// NoopPublisher intentionally drops publications. Use it only when realtime
// delivery is disabled or not wired; it must be explicit, not a silent fallback.
type NoopPublisher struct{}

// Publish drops the publication and returns nil.
func (NoopPublisher) Publish(context.Context, Publication) error {
	return nil
}
