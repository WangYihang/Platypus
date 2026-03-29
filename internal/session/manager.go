package session

import (
	"strings"
	"sync"
)

// Manager is a thread-safe store of sessions keyed by hash.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

// NewManager creates a new empty session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]Session),
	}
}

// Add registers a session. Overwrites any existing session with the same hash.
func (m *Manager) Add(s Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.GetHash()] = s
}

// Remove deletes a session by hash.
func (m *Manager) Remove(hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hash)
}

// Get returns a session by exact hash.
func (m *Manager) Get(hash string) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[hash]
	return s, ok
}

// FindByHashPrefix returns the first session whose hash starts with prefix.
func (m *Manager) FindByHashPrefix(prefix string) Session {
	if prefix == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	prefix = strings.ToLower(prefix)
	for _, s := range m.sessions {
		if strings.HasPrefix(s.GetHash(), prefix) {
			return s
		}
	}
	return nil
}

// FindByAlias returns the first session whose alias starts with the given prefix.
func (m *Manager) FindByAlias(alias string) Session {
	if alias == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	alias = strings.ToLower(alias)
	for _, s := range m.sessions {
		if strings.HasPrefix(s.GetAlias(), alias) {
			return s
		}
	}
	return nil
}

// All returns a snapshot slice of all sessions.
func (m *Manager) All() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// Count returns the number of sessions.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
