package realtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/middleware"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

const (
	TypeConnectedV1  = "realtime.connected.v1"
	TypePingV1       = "realtime.ping.v1"
	TypePongV1       = "realtime.pong.v1"
	TypeSubscribeV1  = "realtime.subscribe.v1"
	TypeSubscribedV1 = "realtime.subscribed.v1"
	TypeErrorV1      = "realtime.error.v1"
)

// Service owns the minimal realtime envelope policy. It does not know Gin or
// the concrete WebSocket library.
type Service struct {
	heartbeatInterval time.Duration
	now               func() time.Time
}

// NewService creates the realtime service.
func NewService(heartbeatInterval time.Duration) *Service {
	if heartbeatInterval <= 0 {
		heartbeatInterval = 25 * time.Second
	}
	return &Service{
		heartbeatInterval: heartbeatInterval,
		now:               time.Now,
	}
}

// HeartbeatInterval returns the server heartbeat interval advertised to clients.
func (s *Service) HeartbeatInterval() time.Duration {
	if s == nil || s.heartbeatInterval <= 0 {
		return 25 * time.Second
	}
	return s.heartbeatInterval
}

// SessionKey builds the in-process connection key for one authenticated admin
// session. It is local state only; distributed fan-out will use Redis later.
func (s *Service) SessionKey(identity *middleware.AuthIdentity) string {
	if identity == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", identity.Platform, identity.UserID, identity.SessionID)
}

// ConnectedEnvelope builds the initial authenticated connection event.
func (s *Service) ConnectedEnvelope(identity *middleware.AuthIdentity, requestID string) (platformrealtime.Envelope, error) {
	data := map[string]any{
		"user_id":               identity.UserID,
		"platform":              identity.Platform,
		"heartbeat_interval_ms": s.HeartbeatInterval().Milliseconds(),
	}
	return platformrealtime.NewEnvelope(TypeConnectedV1, requestID, data)
}

// HandleClientEnvelope handles one client envelope and returns an optional
// server reply.
func (s *Service) HandleClientEnvelope(identity *middleware.AuthIdentity, envelope platformrealtime.Envelope) (*platformrealtime.Envelope, error) {
	switch envelope.Type {
	case TypePingV1:
		return s.pongEnvelope(envelope.RequestID)
	case TypeSubscribeV1:
		return s.subscribeEnvelope(identity, envelope)
	default:
		return errorEnvelope(envelope.RequestID, 400, "unsupported realtime message type")
	}
}

func (s *Service) pongEnvelope(requestID string) (*platformrealtime.Envelope, error) {
	reply, err := platformrealtime.NewEnvelope(TypePongV1, requestID, map[string]any{
		"server_time": s.now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

type subscribePayload struct {
	Topics []string `json:"topics"`
}

func (s *Service) subscribeEnvelope(identity *middleware.AuthIdentity, envelope platformrealtime.Envelope) (*platformrealtime.Envelope, error) {
	if identity == nil || identity.UserID <= 0 || identity.SessionID <= 0 || strings.TrimSpace(identity.Platform) == "" {
		return errorEnvelope(envelope.RequestID, 401, "unauthenticated realtime session")
	}

	var payload subscribePayload
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		return errorEnvelope(envelope.RequestID, 400, "invalid subscribe payload")
	}
	if len(payload.Topics) == 0 {
		return errorEnvelope(envelope.RequestID, 400, "topics is required")
	}

	allowed := allowedTopics(identity)
	accepted := make([]string, 0, len(payload.Topics))
	seen := make(map[string]struct{}, len(payload.Topics))
	for _, rawTopic := range payload.Topics {
		topic := strings.TrimSpace(rawTopic)
		if topic == "" {
			return errorEnvelope(envelope.RequestID, 400, "topic is required")
		}
		if _, ok := allowed[topic]; !ok {
			return errorEnvelope(envelope.RequestID, 403, "无订阅权限")
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		accepted = append(accepted, topic)
	}

	reply, err := platformrealtime.NewEnvelope(TypeSubscribedV1, envelope.RequestID, map[string]any{
		"topics": accepted,
	})
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func allowedTopics(identity *middleware.AuthIdentity) map[string]struct{} {
	return map[string]struct{}{
		fmt.Sprintf("user:%d", identity.UserID):       {},
		fmt.Sprintf("session:%d", identity.SessionID): {},
		"platform:" + identity.Platform:               {},
	}
}

func errorEnvelope(requestID string, code int, message string) (*platformrealtime.Envelope, error) {
	reply, err := platformrealtime.NewEnvelope(TypeErrorV1, requestID, map[string]any{
		"code": code,
		"msg":  message,
	})
	if err != nil {
		return nil, err
	}
	return &reply, nil
}
