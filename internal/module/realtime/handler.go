package realtime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	platformrealtime "admin_back_go/internal/platform/realtime"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

const defaultSendBuffer = 16

// Handler owns the HTTP/WebSocket boundary for admin realtime.
type Handler struct {
	service    *Service
	upgrader   *platformrealtime.Upgrader
	manager    *platformrealtime.Manager
	logger     *slog.Logger
	enabled    bool
	sendBuffer int
}

// Option customizes the realtime handler.
type Option func(*Handler)

// WithEnabled explicitly enables or disables WebSocket upgrades.
func WithEnabled(enabled bool) Option {
	return func(h *Handler) {
		h.enabled = enabled
	}
}

// WithSendBuffer sets the bounded outbound queue length per connection.
func WithSendBuffer(size int) Option {
	return func(h *Handler) {
		if size > 0 {
			h.sendBuffer = size
		}
	}
}

// NewHandler creates a realtime handler.
func NewHandler(service *Service, upgrader *platformrealtime.Upgrader, manager *platformrealtime.Manager, logger *slog.Logger, options ...Option) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if manager == nil {
		manager = platformrealtime.NewManager()
	}
	if upgrader == nil {
		upgrader = platformrealtime.NewUpgrader(nil)
	}
	handler := &Handler{
		service:    service,
		upgrader:   upgrader,
		manager:    manager,
		logger:     logger,
		enabled:    true,
		sendBuffer: defaultSendBuffer,
	}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler
}

// WebSocket upgrades an authenticated HTTP request and serves the minimal
// connect/ping/pong lifecycle.
func (h *Handler) WebSocket(c *gin.Context) {
	if h == nil || !h.enabled {
		response.Abort(c, apperror.NewKey(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "realtime.disabled", nil, "Realtime未启用"))
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil {
		response.Abort(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Abort(c, nil)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request)
	if err != nil {
		h.logger.WarnContext(c.Request.Context(), "websocket upgrade failed", "error", err)
		return
	}

	session := platformrealtime.NewSession(conn, platformrealtime.SessionOptions{
		SendBuffer:   h.sendBuffer,
		WriteWait:    5 * time.Second,
		PongWait:     2 * h.service.HeartbeatInterval(),
		PingInterval: h.service.HeartbeatInterval(),
	})
	unregister := h.manager.Register(h.service.SessionKey(identity), session)
	defer unregister()

	connected, err := h.service.ConnectedEnvelope(identity, c.GetHeader(middleware.HeaderRequestID))
	if err != nil {
		h.logger.WarnContext(c.Request.Context(), "websocket connected event failed", "error", err)
		return
	}
	if err := session.Send(connected); err != nil {
		h.logger.WarnContext(c.Request.Context(), "websocket connected enqueue failed", "error", err)
		return
	}

	err = session.Serve(c.Request.Context(), func(ctx context.Context, envelope platformrealtime.Envelope) (*platformrealtime.Envelope, error) {
		return h.service.HandleClientEnvelope(identity, envelope)
	})
	if err != nil && !errors.Is(err, platformrealtime.ErrConnectionClosed) {
		h.logger.DebugContext(c.Request.Context(), "websocket session ended", "error", err)
	}
}
