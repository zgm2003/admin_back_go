package realtime

import (
	"context"
	"errors"
	"strings"
)

var ErrPublicationTargetRequired = errors.New("realtime publication target required")

// Publication is one server-side realtime message delivery request. It is an
// internal contract; business modules publish envelopes without knowing whether
// delivery is local-only, Redis Pub/Sub, or another fan-out implementation.
type Publication struct {
	SessionKey string   `json:"session_key,omitempty"`
	Platform   string   `json:"platform,omitempty"`
	UserID     int64    `json:"user_id,omitempty"`
	Envelope   Envelope `json:"envelope"`
}

// Publisher sends realtime envelopes to connected clients. Implementations may
// be local-process, Redis Pub/Sub, Redis Streams, or a test/no-op publisher.
type Publisher interface {
	Publish(context.Context, Publication) error
}

// LocalPublisher publishes to the in-process Manager only.
type LocalPublisher struct {
	manager *Manager
}

// NewLocalPublisher creates a local-process realtime publisher.
func NewLocalPublisher(manager *Manager) *LocalPublisher {
	return &LocalPublisher{manager: manager}
}

// Publish sends one envelope to a local session key or all local sessions for
// a platform user.
func (p *LocalPublisher) Publish(ctx context.Context, publication Publication) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.manager == nil {
		return ErrSessionNotFound
	}
	key := strings.TrimSpace(publication.SessionKey)
	if key != "" {
		return p.manager.Send(key, publication.Envelope)
	}
	platform := strings.TrimSpace(publication.Platform)
	if platform == "" || publication.UserID <= 0 {
		return ErrPublicationTargetRequired
	}
	return p.manager.SendToUser(platform, publication.UserID, publication.Envelope)
}

// NoopPublisher intentionally drops publications. Use it only when realtime
// delivery is disabled or not wired; it must be explicit, not a silent fallback.
type NoopPublisher struct{}

// Publish drops the publication and returns nil.
func (NoopPublisher) Publish(context.Context, Publication) error {
	return nil
}
