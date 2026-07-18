package realtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/middleware"
)

// Service owns the realtime event registry and authenticated client-control
// policy. It does not know Gin or the concrete WebSocket library.
type Service struct {
	heartbeatInterval time.Duration
	now               func() time.Time
	registry          *EventRegistry
	events            EventReader
}

var ErrEventReaderNotConfigured = errors.New("realtime event reader is not configured")

type EventReader interface {
	ResumeUser(context.Context, ResumeQuery) (*ResumeResult, error)
}

type ServiceOption func(*Service)

func WithEventReader(reader EventReader) ServiceOption {
	return func(service *Service) {
		service.events = reader
	}
}

// NewService creates the realtime service.
func NewService(heartbeatInterval time.Duration, options ...ServiceOption) *Service {
	if heartbeatInterval <= 0 {
		heartbeatInterval = 25 * time.Second
	}
	service := &Service{
		heartbeatInterval: heartbeatInterval,
		now:               time.Now,
		registry:          DefaultRegistry(),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// HeartbeatInterval returns the server heartbeat interval advertised to clients.
func (s *Service) HeartbeatInterval() time.Duration {
	if s == nil || s.heartbeatInterval <= 0 {
		return 25 * time.Second
	}
	return s.heartbeatInterval
}

// SessionKey builds the in-process connection key for one authenticated admin
// session.
func (s *Service) SessionKey(identity *middleware.AuthIdentity) string {
	if identity == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", identity.Platform, identity.UserID, identity.SessionID)
}

// ConnectedEnvelope builds the initial authenticated connection event.
func (s *Service) ConnectedEnvelope(identity *middleware.AuthIdentity, requestID string) (infrarealtime.Envelope, error) {
	if !validIdentity(identity) {
		return infrarealtime.Envelope{}, fmt.Errorf("invalid realtime identity")
	}
	return s.eventRegistry().NewEphemeral(TypeConnectedV1, requestID, ConnectedPayload{
		UserID: identity.UserID, Platform: identity.Platform, HeartbeatIntervalMS: s.HeartbeatInterval().Milliseconds(),
	}, s.currentTime())
}

// HandleClientEnvelope validates one closed client event and returns zero or
// more ordered replies. Subscription controls mutate only this session.
func (s *Service) HandleClientEnvelope(ctx context.Context, identity *middleware.AuthIdentity, session *infrarealtime.Session, envelope infrarealtime.Envelope) ([]infrarealtime.Envelope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	definition, ok := s.eventRegistry().Definition(envelope.Type)
	if !ok || (definition.Direction != DirectionClient && definition.Direction != DirectionBidirectional) {
		return s.errorReplies(envelope.RequestID, 400, "unsupported realtime message type")
	}
	payload, err := s.eventRegistry().DecodePayload(envelope.Type, envelope.Data)
	if err != nil {
		return s.errorReplies(envelope.RequestID, 400, "invalid realtime payload")
	}

	switch envelope.Type {
	case TypePingV1:
		reply, err := s.eventRegistry().NewEphemeral(TypePongV1, envelope.RequestID, PongPayload{
			ServerTime: s.currentTime().Format(time.RFC3339Nano),
		}, s.currentTime())
		return singleReply(reply, err)
	case TypeSubscribeV1:
		subscribe, ok := payload.(*SubscribePayload)
		if !ok {
			return s.errorReplies(envelope.RequestID, 400, "invalid subscribe payload")
		}
		return s.subscribeReplies(identity, session, envelope.RequestID, *subscribe)
	case TypeResumeV1:
		resume, ok := payload.(*ResumePayload)
		if !ok || resume.AfterSequence == nil {
			return s.errorReplies(envelope.RequestID, 400, "invalid resume payload")
		}
		return s.resumeReplies(ctx, identity, envelope.RequestID, *resume.AfterSequence)
	default:
		return s.errorReplies(envelope.RequestID, 400, "unsupported realtime message type")
	}
}

func (s *Service) resumeReplies(ctx context.Context, identity *middleware.AuthIdentity, requestID string, afterSequence uint64) ([]infrarealtime.Envelope, error) {
	if !validIdentity(identity) {
		return s.errorReplies(requestID, 401, "unauthenticated realtime session")
	}
	if s == nil || s.events == nil {
		return nil, ErrEventReaderNotConfigured
	}
	result, err := s.events.ResumeUser(ctx, ResumeQuery{UserID: identity.UserID, AfterSequence: afterSequence, Limit: MaxResumeLimit, Now: s.currentTime()})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("realtime resume returned nil result")
	}
	if result.ResyncRequired {
		reply, err := s.eventRegistry().NewEphemeral(TypeResyncRequiredV1, requestID, ResyncRequiredPayload{
			LatestSequence: result.LatestSequence,
		}, s.currentTime())
		return singleReply(reply, err)
	}
	replies := make([]infrarealtime.Envelope, 0, len(result.Events))
	previous := afterSequence
	for _, event := range result.Events {
		if event.Sequence <= previous {
			return nil, fmt.Errorf("realtime resume sequence is not strictly increasing")
		}
		reply, err := event.Envelope(s.eventRegistry())
		if err != nil {
			return nil, err
		}
		replies = append(replies, reply)
		previous = event.Sequence
	}
	return replies, nil
}

func (s *Service) subscribeReplies(identity *middleware.AuthIdentity, session *infrarealtime.Session, requestID string, payload SubscribePayload) ([]infrarealtime.Envelope, error) {
	if !validIdentity(identity) {
		return s.errorReplies(requestID, 401, "unauthenticated realtime session")
	}
	if session == nil {
		return nil, fmt.Errorf("realtime session is required")
	}

	allowed := allowedTopics(identity)
	accepted := make([]string, 0, len(payload.Topics))
	for _, rawTopic := range payload.Topics {
		topic := strings.TrimSpace(rawTopic)
		if _, ok := allowed[topic]; !ok {
			return s.errorReplies(requestID, 403, "无订阅权限")
		}
		accepted = append(accepted, topic)
	}
	session.ReplaceTopics(accepted)
	reply, err := s.eventRegistry().NewEphemeral(TypeSubscribedV1, requestID, SubscribedPayload{Topics: accepted}, s.currentTime())
	return singleReply(reply, err)
}

func allowedTopics(identity *middleware.AuthIdentity) map[string]struct{} {
	return map[string]struct{}{
		fmt.Sprintf("user:%d", identity.UserID):       {},
		fmt.Sprintf("session:%d", identity.SessionID): {},
		"platform:" + identity.Platform:               {},
	}
}

func (s *Service) errorReplies(requestID string, code int, message string) ([]infrarealtime.Envelope, error) {
	reply, err := s.eventRegistry().NewEphemeral(TypeErrorV1, requestID, ErrorPayload{Code: code, Msg: message}, s.currentTime())
	return singleReply(reply, err)
}

func singleReply(reply infrarealtime.Envelope, err error) ([]infrarealtime.Envelope, error) {
	if err != nil {
		return nil, err
	}
	return []infrarealtime.Envelope{reply}, nil
}

func (s *Service) eventRegistry() *EventRegistry {
	if s == nil || s.registry == nil {
		return DefaultRegistry()
	}
	return s.registry
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now()
}

func validIdentity(identity *middleware.AuthIdentity) bool {
	return identity != nil && identity.UserID > 0 && identity.SessionID > 0 && strings.TrimSpace(identity.Platform) == "admin"
}
