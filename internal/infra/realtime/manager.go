package realtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"admin_back_go/internal/telemetry"
)

// Manager owns local realtime sessions for this process. Multi-node fan-out is
// intentionally outside this type and uses Publisher implementations.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	recorder telemetry.Recorder
}

type Option func(*Manager)

func WithTelemetry(recorder telemetry.Recorder) Option {
	return func(manager *Manager) {
		if recorder != nil {
			manager.recorder = recorder
		}
	}
}

// NewManager creates an in-process realtime session manager.
func NewManager(options ...Option) *Manager {
	manager := &Manager{sessions: make(map[string]*Session), recorder: telemetry.Noop()}
	for _, option := range options {
		if option != nil {
			option(manager)
		}
	}
	return manager
}

// Register stores a session by key and closes any old session using that key.
// The returned function removes exactly this session, not a later replacement.
func (m *Manager) Register(key string, session *Session) func() {
	if m == nil || key == "" || session == nil {
		return func() {}
	}

	m.mu.Lock()
	old := m.sessions[key]
	m.sessions[key] = session
	m.mu.Unlock()

	if old != nil && old != session {
		_ = old.Close()
		m.record("reconnect", "ok")
	} else {
		m.record("connect", "ok")
	}

	return func() {
		m.mu.Lock()
		current := m.sessions[key]
		if current == session {
			delete(m.sessions, key)
		}
		m.mu.Unlock()

		if current == session {
			_ = session.Close()
			m.record("drop", "closed")
		}
	}
}

// Send enqueues a message to one registered session.
func (m *Manager) Send(key string, envelope Envelope) error {
	if m == nil {
		return ErrSessionNotFound
	}
	m.mu.RLock()
	session := m.sessions[key]
	m.mu.RUnlock()
	if session == nil {
		return ErrSessionNotFound
	}
	err := session.Send(envelope)
	if errors.Is(err, ErrSendQueueFull) {
		m.record("send_pressure", "dropped")
	}
	return err
}

// SendToUser enqueues a message to every local session for one platform user.
func (m *Manager) SendToUser(platform string, userID int64, envelope Envelope) error {
	if m == nil {
		return ErrSessionNotFound
	}
	platform = strings.TrimSpace(platform)
	if platform == "" || userID <= 0 {
		return ErrPublicationTargetRequired
	}

	prefix := fmt.Sprintf("%s:%d:", platform, userID)
	m.mu.RLock()
	sessions := make([]*Session, 0)
	for key, session := range m.sessions {
		if strings.HasPrefix(key, prefix) && session != nil {
			sessions = append(sessions, session)
		}
	}
	m.mu.RUnlock()

	if len(sessions) == 0 {
		return ErrSessionNotFound
	}
	var err error
	for _, session := range sessions {
		if sendErr := session.Send(envelope); sendErr != nil {
			if errors.Is(sendErr, ErrSendQueueFull) {
				m.record("send_pressure", "dropped")
			}
			err = errors.Join(err, sendErr)
		}
	}
	return err
}

func (m *Manager) record(operation string, outcome string) {
	if m == nil {
		return
	}
	recorder := m.recorder
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	recorder.Count("realtime.events", 1, telemetry.Attributes{
		"realtime.operation": operation,
		"realtime.transport": "websocket",
		"realtime.outcome":   outcome,
	})
}

// Count returns the current number of locally registered sessions.
func (m *Manager) Count() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// CloseAll closes and removes every local session.
func (m *Manager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for key, session := range m.sessions {
		sessions = append(sessions, session)
		delete(m.sessions, key)
	}
	m.mu.Unlock()

	for _, session := range sessions {
		_ = session.Close()
	}
}
