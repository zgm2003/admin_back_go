package realtime

import (
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrUpgradeFailed = errors.New("websocket upgrade failed")
	ErrReadFailed    = errors.New("websocket read failed")
	ErrWriteFailed   = errors.New("websocket write failed")
)

// Upgrader is the thin project wrapper around gorilla/websocket. It exists so
// the rest of the codebase does not depend on gorilla types directly.
type Upgrader struct {
	upgrader websocket.Upgrader
}

// NewUpgrader creates a WebSocket upgrader with the provided origin policy.
func NewUpgrader(checkOrigin func(*http.Request) bool) *Upgrader {
	return &Upgrader{
		upgrader: websocket.Upgrader{
			CheckOrigin: checkOrigin,
		},
	}
}

// Upgrade upgrades a Gin/http request to WebSocket.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if u == nil {
		u = NewUpgrader(nil)
	}
	conn, err := u.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, errors.Join(ErrUpgradeFailed, err)
	}
	return &Conn{conn: conn}, nil
}

// Conn hides gorilla.Conn behind the project realtime boundary.
type Conn struct {
	conn *websocket.Conn
}

// ReadEnvelope reads the next client JSON envelope.
func (c *Conn) ReadEnvelope() (Envelope, error) {
	_, payload, err := c.conn.ReadMessage()
	if err != nil {
		return Envelope{}, errors.Join(ErrReadFailed, err)
	}
	envelope, err := DecodeEnvelope(payload)
	if err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// WriteEnvelope writes one server JSON envelope. Callers must serialize writes;
// the connection manager owns that with a bounded send channel.
func (c *Conn) WriteEnvelope(envelope Envelope) error {
	payload, err := EncodeEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return errors.Join(ErrWriteFailed, err)
	}
	return nil
}

// SetReadDeadline updates the underlying read deadline.
func (c *Conn) SetReadDeadline(deadline time.Time) error {
	return c.conn.SetReadDeadline(deadline)
}

// SetWriteDeadline updates the underlying write deadline.
func (c *Conn) SetWriteDeadline(deadline time.Time) error {
	return c.conn.SetWriteDeadline(deadline)
}

// SetPongHandler sets the handler for pong control messages.
func (c *Conn) SetPongHandler(handler func(string) error) error {
	c.conn.SetPongHandler(handler)
	return nil
}

// WritePing writes a WebSocket ping control message.
func (c *Conn) WritePing(deadline time.Time) error {
	if err := c.conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
		return errors.Join(ErrWriteFailed, err)
	}
	return nil
}

// Close closes the WebSocket connection.
func (c *Conn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
