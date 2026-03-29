package listener

import (
	"strings"
	"sync"
)

// Manager is a thread-safe store of listeners keyed by hash.
type Manager struct {
	mu        sync.RWMutex
	listeners map[string]Listener
}

// NewManager creates a new empty listener manager.
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string]Listener),
	}
}

// Add registers a listener.
func (m *Manager) Add(l Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners[l.GetHash()] = l
}

// Remove deletes a listener by hash and stops it.
func (m *Manager) Remove(hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.listeners[hash]; ok {
		l.Stop()
		delete(m.listeners, hash)
	}
}

// Get returns a listener by exact hash.
func (m *Manager) Get(hash string) (Listener, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.listeners[hash]
	return l, ok
}

// FindByHashPrefix returns the first listener whose hash starts with prefix.
func (m *Manager) FindByHashPrefix(prefix string) Listener {
	if prefix == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	prefix = strings.ToLower(prefix)
	for _, l := range m.listeners {
		if strings.HasPrefix(l.GetHash(), prefix) {
			return l
		}
	}
	return nil
}

// All returns a snapshot slice of all listeners.
func (m *Manager) All() []Listener {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Listener, 0, len(m.listeners))
	for _, l := range m.listeners {
		result = append(result, l)
	}
	return result
}

// Count returns the number of listeners.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.listeners)
}
