package agent

import (
	"net"
	"sync"
)

// ConnMap is a thread-safe map of net.Conn pointers keyed by token.
type ConnMap struct {
	mu sync.RWMutex
	m  map[string]*net.Conn
}

// NewConnMap creates a new empty ConnMap.
func NewConnMap() *ConnMap {
	return &ConnMap{m: make(map[string]*net.Conn)}
}

func (cm *ConnMap) Get(key string) (*net.Conn, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	c, ok := cm.m[key]
	return c, ok
}

func (cm *ConnMap) Set(key string, c *net.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.m[key] = c
}

func (cm *ConnMap) Delete(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.m, key)
}

// GetAndDelete atomically retrieves and deletes a connection.
func (cm *ConnMap) GetAndDelete(key string) (*net.Conn, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, ok := cm.m[key]
	if ok {
		delete(cm.m, key)
	}
	return c, ok
}
