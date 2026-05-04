package realtime

import "sync"

// Manager owns local realtime sessions for this process. Multi-node fan-out is
// intentionally outside this type and will use Redis later.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager creates an in-process realtime session manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
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
	return session.Send(envelope)
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
