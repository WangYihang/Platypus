package agent

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// AgentProcess represents a PTY-attached managed process on the agent host.
type AgentProcess struct {
	Ptmx       *os.File
	WindowSize *pty.Winsize
	Process    *exec.Cmd
}

// ProcessMap is a thread-safe map of processes keyed by token.
type ProcessMap struct {
	mu sync.RWMutex
	m  map[string]*AgentProcess
}

// NewProcessMap creates a new empty ProcessMap.
func NewProcessMap() *ProcessMap {
	return &ProcessMap{m: make(map[string]*AgentProcess)}
}

func (pm *ProcessMap) Get(key string) (*AgentProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.m[key]
	return p, ok
}

func (pm *ProcessMap) Set(key string, p *AgentProcess) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.m[key] = p
}

func (pm *ProcessMap) Delete(key string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.m, key)
}
