package realtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrConnectionClosed = errors.New("realtime connection closed")
	ErrSendQueueFull    = errors.New("realtime send queue full")
	ErrSessionClosed    = errors.New("realtime session closed")
	ErrSessionNotFound  = errors.New("realtime session not found")
)

const (
	defaultSendBuffer   = 16
	defaultWriteWait    = 5 * time.Second
	defaultPongWait     = 60 * time.Second
	defaultPingInterval = 25 * time.Second
)

// EnvelopeHandler handles one client envelope and returns an optional reply.
type EnvelopeHandler func(context.Context, Envelope) (*Envelope, error)

// SessionOptions controls a single WebSocket session lifecycle.
type SessionOptions struct {
	SendBuffer   int
	WriteWait    time.Duration
	PongWait     time.Duration
	PingInterval time.Duration
}

// Session owns one WebSocket connection, its bounded send queue, and pump
// lifecycle. Business code must not write to Conn directly.
type Session struct {
	conn    *Conn
	send    chan Envelope
	done    chan struct{}
	options SessionOptions

	mu     sync.Mutex
	closed bool
}

// NewSession creates a session with a bounded send queue.
func NewSession(conn *Conn, options SessionOptions) *Session {
	options = normalizeSessionOptions(options)
	return &Session{
		conn:    conn,
		send:    make(chan Envelope, options.SendBuffer),
		done:    make(chan struct{}),
		options: options,
	}
}

// Send enqueues one outbound envelope. It never blocks forever: a full queue
// closes the session so slow clients cannot grow memory without bound.
func (s *Session) Send(envelope Envelope) error {
	if s == nil {
		return ErrSessionClosed
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}

	select {
	case s.send <- envelope:
		s.mu.Unlock()
		return nil
	default:
		conn := s.closeLocked()
		s.mu.Unlock()
		return closeConnWithError(conn, ErrSendQueueFull)
	}
}

// Done is closed when the session is closed.
func (s *Session) Done() <-chan struct{} {
	if s == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return s.done
}

// Close closes the session and its underlying connection.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	conn := s.closeLocked()
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

// Serve runs the read and write pumps until the client disconnects, context is
// cancelled, or a pump returns an error.
func (s *Session) Serve(ctx context.Context, handler EnvelopeHandler) error {
	if s == nil || s.conn == nil {
		return ErrConnectionClosed
	}
	if handler == nil {
		handler = func(context.Context, Envelope) (*Envelope, error) { return nil, nil }
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- s.readPump(ctx, handler) }()
	go func() { errCh <- s.writePump(ctx) }()

	err := <-errCh
	cancel()
	_ = s.Close()

	select {
	case other := <-errCh:
		if err == nil {
			err = other
		}
	case <-time.After(100 * time.Millisecond):
	}

	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (s *Session) readPump(ctx context.Context, handler EnvelopeHandler) error {
	if s.options.PongWait > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.options.PongWait))
		_ = s.conn.SetPongHandler(func(string) error {
			return s.conn.SetReadDeadline(time.Now().Add(s.options.PongWait))
		})
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return ErrConnectionClosed
		default:
		}

		envelope, err := s.conn.ReadEnvelope()
		if err != nil {
			return errors.Join(ErrConnectionClosed, err)
		}

		reply, err := handler(ctx, envelope)
		if err != nil {
			return err
		}
		if reply == nil {
			continue
		}
		if err := s.Send(*reply); err != nil {
			return err
		}
	}
}

func (s *Session) writePump(ctx context.Context) error {
	ticker := time.NewTicker(s.options.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return ErrConnectionClosed
		case envelope := <-s.send:
			if err := s.conn.SetWriteDeadline(time.Now().Add(s.options.WriteWait)); err != nil {
				return err
			}
			if err := s.conn.WriteEnvelope(envelope); err != nil {
				return err
			}
		case <-ticker.C:
			deadline := time.Now().Add(s.options.WriteWait)
			if err := s.conn.WritePing(deadline); err != nil {
				return errors.Join(ErrConnectionClosed, err)
			}
		}
	}
}

func (s *Session) closeLocked() *Conn {
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.done)
	return s.conn
}

func normalizeSessionOptions(options SessionOptions) SessionOptions {
	if options.SendBuffer <= 0 {
		options.SendBuffer = defaultSendBuffer
	}
	if options.WriteWait <= 0 {
		options.WriteWait = defaultWriteWait
	}
	if options.PongWait <= 0 {
		options.PongWait = defaultPongWait
	}
	if options.PingInterval <= 0 {
		options.PingInterval = defaultPingInterval
	}
	return options
}

func closeConnWithError(conn *Conn, err error) error {
	if conn != nil {
		return errors.Join(err, conn.Close())
	}
	return err
}
