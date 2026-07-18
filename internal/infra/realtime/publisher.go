package realtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/telemetry"
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

// EnvelopeValidator lets the runtime inject its closed business event
// registry without making the transport package depend on business modules.
type EnvelopeValidator func(Envelope) error

type instrumentedPublisher struct {
	delegate  Publisher
	transport string
	recorder  telemetry.Recorder
}

func InstrumentPublisher(delegate Publisher, transport string, recorder telemetry.Recorder) Publisher {
	if delegate == nil {
		return nil
	}
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	return &instrumentedPublisher{delegate: delegate, transport: strings.TrimSpace(transport), recorder: recorder}
}

func (publisher *instrumentedPublisher) Publish(ctx context.Context, publication Publication) error {
	startedAt := time.Now()
	err := publisher.delegate.Publish(ctx, publication)
	outcome := "ok"
	if publisher.transport == "noop" || errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrTopicNotSubscribed) || errors.Is(err, ErrSendQueueFull) {
		outcome = "dropped"
	} else if err != nil {
		outcome = "error"
	}
	attributes := telemetry.Attributes{
		"realtime.operation": "publish",
		"realtime.transport": publisher.transport,
		"realtime.outcome":   outcome,
	}
	publisher.recorder.Count("realtime.publications", 1, attributes)
	publisher.recorder.Observe("realtime.publish.duration_seconds", time.Since(startedAt).Seconds(), attributes)
	return err
}

// LocalPublisher publishes to the in-process Manager only.
type LocalPublisher struct {
	manager   *Manager
	validator EnvelopeValidator
}

// NewLocalPublisher creates a local-process realtime publisher.
func NewLocalPublisher(manager *Manager, validators ...EnvelopeValidator) *LocalPublisher {
	return &LocalPublisher{manager: manager, validator: firstEnvelopeValidator(validators)}
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
	platform := strings.TrimSpace(publication.Platform)
	if key == "" && (platform == "" || publication.UserID <= 0) {
		return ErrPublicationTargetRequired
	}
	if err := ValidateServerEnvelope(publication.Envelope); err != nil {
		return err
	}
	if p.validator != nil {
		if err := p.validator(publication.Envelope); err != nil {
			return err
		}
	}
	if key != "" {
		return p.manager.Send(key, publication.Envelope)
	}
	return p.manager.SendToUser(platform, publication.UserID, publication.Envelope)
}

func firstEnvelopeValidator(validators []EnvelopeValidator) EnvelopeValidator {
	for _, validator := range validators {
		if validator != nil {
			return validator
		}
	}
	return nil
}

// NoopPublisher intentionally drops publications. Use it only when realtime
// delivery is disabled or not wired; it must be explicit, not a silent fallback.
type NoopPublisher struct{}

// Publish drops the publication and returns nil.
func (NoopPublisher) Publish(context.Context, Publication) error {
	return nil
}

var _ Publisher = (*instrumentedPublisher)(nil)
